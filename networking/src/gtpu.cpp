#include "gtpu.h"

#include <cstdio>
#include <cstring>

#ifdef AUSF_PLATFORM_WINDOWS
#  pragma comment(lib, "ws2_32.lib")
#endif

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// Header codec
// ---------------------------------------------------------------------------

std::vector<uint8_t> encodeGTPUHeader(const GTPUHeader& hdr) {
    const bool has_opt = hdr.e || hdr.s || hdr.pn;
    std::vector<uint8_t> buf;
    buf.reserve(has_opt ? 12u : 8u);

    // Octet 1: Ver(3) | PT | spare | E | S | PN
    const uint8_t oct0 = static_cast<uint8_t>(
          ((hdr.version & 0x7u) << 5u)
        | (hdr.pt ? 0x10u : 0u)
        | (hdr.e  ? 0x04u : 0u)
        | (hdr.s  ? 0x02u : 0u)
        | (hdr.pn ? 0x01u : 0u));
    buf.push_back(oct0);

    // Octet 2: Message type
    buf.push_back(static_cast<uint8_t>(hdr.message_type));

    // Octets 3-4: Length (big-endian)
    buf.push_back(static_cast<uint8_t>((hdr.length >> 8u) & 0xFFu));
    buf.push_back(static_cast<uint8_t>( hdr.length        & 0xFFu));

    // Octets 5-8: TEID (big-endian)
    buf.push_back(static_cast<uint8_t>((hdr.teid >> 24u) & 0xFFu));
    buf.push_back(static_cast<uint8_t>((hdr.teid >> 16u) & 0xFFu));
    buf.push_back(static_cast<uint8_t>((hdr.teid >>  8u) & 0xFFu));
    buf.push_back(static_cast<uint8_t>( hdr.teid         & 0xFFu));

    if (has_opt) {
        buf.push_back(static_cast<uint8_t>((hdr.sequence_number >> 8u) & 0xFFu));
        buf.push_back(static_cast<uint8_t>( hdr.sequence_number        & 0xFFu));
        buf.push_back(hdr.npdu_number);
        buf.push_back(hdr.next_ext_hdr);
    }

    return buf;
}

std::optional<GTPUHeader> decodeGTPUHeader(const std::vector<uint8_t>& buf) {
    if (buf.size() < 8u) return std::nullopt;

    GTPUHeader hdr;
    hdr.version = (buf[0] >> 5u) & 0x7u;
    if (hdr.version != 1u) return std::nullopt;

    hdr.pt = (buf[0] & 0x10u) != 0u;
    hdr.e  = (buf[0] & 0x04u) != 0u;
    hdr.s  = (buf[0] & 0x02u) != 0u;
    hdr.pn = (buf[0] & 0x01u) != 0u;

    hdr.message_type = static_cast<GTPUMessageType>(buf[1]);

    hdr.length = static_cast<uint16_t>(
        (static_cast<uint16_t>(buf[2]) << 8u) | static_cast<uint16_t>(buf[3]));

    hdr.teid = (static_cast<uint32_t>(buf[4]) << 24u)
             | (static_cast<uint32_t>(buf[5]) << 16u)
             | (static_cast<uint32_t>(buf[6]) <<  8u)
             |  static_cast<uint32_t>(buf[7]);

    if (hdr.e || hdr.s || hdr.pn) {
        if (buf.size() < 12u) return std::nullopt;
        hdr.sequence_number = static_cast<uint16_t>(
            (static_cast<uint16_t>(buf[8]) << 8u) | static_cast<uint16_t>(buf[9]));
        hdr.npdu_number  = buf[10];
        hdr.next_ext_hdr = buf[11];
    }

    return hdr;
}

std::vector<uint8_t> encodeGTPU(GTPUHeader hdr, const std::vector<uint8_t>& payload) {
    const bool has_opt = hdr.e || hdr.s || hdr.pn;
    hdr.length = static_cast<uint16_t>((has_opt ? 4u : 0u) + payload.size());
    std::vector<uint8_t> frame = encodeGTPUHeader(hdr);
    frame.insert(frame.end(), payload.begin(), payload.end());
    return frame;
}

// Overload: GTP-U frame with PDU Session Container extension header (TS 38.415).
// Always emits the 12-byte header (E=true, next_ext_hdr=0x85) + 4-byte PSC ext hdr
// + payload.  hdr.length is set automatically.
std::vector<uint8_t> encodeGTPU(GTPUHeader hdr, const std::vector<uint8_t>& payload,
                                 const PduSessionContainer& psc) {
    // Force extension-header path
    hdr.e            = true;
    hdr.next_ext_hdr = GTPU_EXT_HDR_PDU_SESSION_CONTAINER;
    // length = 4 (opt fields) + 4 (PSC ext hdr) + payload
    hdr.length = static_cast<uint16_t>(4u + 4u + payload.size());

    std::vector<uint8_t> frame = encodeGTPUHeader(hdr);  // 12 bytes

    // PDU Session Container extension header (4 bytes)
    frame.push_back(0x01u);                                             // ext hdr length = 1×4 bytes
    frame.push_back(static_cast<uint8_t>(psc.uplink ? 0x10u : 0x00u)); // PDU type [7:4], spare [3:0]
    frame.push_back(static_cast<uint8_t>(psc.qfi & 0x3Fu));            // spare[7:6] + QFI[5:0]
    frame.push_back(0x00u);                                             // no next ext hdr

    frame.insert(frame.end(), payload.begin(), payload.end());
    return frame;
}

std::optional<GTPUFrame> decodeGTPU(const std::vector<uint8_t>& frame) {
    auto hdr_opt = decodeGTPUHeader(frame);
    if (!hdr_opt) return std::nullopt;

    std::size_t payload_offset = hdr_opt->wireSize();  // 8 or 12

    GTPUFrame result;
    result.header = std::move(*hdr_opt);

    // Parse PDU Session Container extension header if present
    if (result.header.e && result.header.next_ext_hdr == GTPU_EXT_HDR_PDU_SESSION_CONTAINER) {
        // Expect 4-byte PSC ext hdr (length byte = 0x01 → 1×4 bytes)
        if (frame.size() < payload_offset + 4u) return std::nullopt;
        const uint8_t ext_len = frame[payload_offset];
        if (ext_len != 0x01u) return std::nullopt;  // unexpected length

        PduSessionContainer psc;
        psc.uplink = ((frame[payload_offset + 1u] >> 4u) & 0x0Fu) == 1u;
        psc.qfi    = frame[payload_offset + 2u] & 0x3Fu;
        result.pdu_session_container = psc;
        payload_offset += 4u;  // skip the 4-byte ext hdr
    }

    if (frame.size() < payload_offset) return std::nullopt;
    result.payload = std::vector<uint8_t>(
        frame.begin() + static_cast<std::ptrdiff_t>(payload_offset), frame.end());
    return result;
}

// ---------------------------------------------------------------------------
// GTPUSocket
// ---------------------------------------------------------------------------

GTPUSocket::GTPUSocket(uint16_t port) : port_(port) {
#ifdef AUSF_PLATFORM_WINDOWS
    WSADATA wsa{};
    WSAStartup(MAKEWORD(2, 2), &wsa);
#endif
}

GTPUSocket::~GTPUSocket() {
    close();
#ifdef AUSF_PLATFORM_WINDOWS
    WSACleanup();
#endif
}

bool GTPUSocket::open() {
    fd_ = ::socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (fd_ == INVALID_SOCKET_VAL) {
        std::perror("gtpu_socket: socket()");
        return false;
    }

    int optval = 1;
#ifdef AUSF_PLATFORM_WINDOWS
    ::setsockopt(fd_, SOL_SOCKET, SO_REUSEADDR,
                 reinterpret_cast<const char*>(&optval), sizeof(optval));
#else
    ::setsockopt(fd_, SOL_SOCKET, SO_REUSEADDR, &optval, sizeof(optval));
#endif

    struct sockaddr_in addr{};
    addr.sin_family      = AF_INET;
    addr.sin_port        = htons(port_);
    addr.sin_addr.s_addr = htonl(INADDR_ANY);

    if (::bind(fd_, reinterpret_cast<struct sockaddr*>(&addr), sizeof(addr)) != 0) {
        std::perror("gtpu_socket: bind()");
        close();
        return false;
    }
    return true;
}

void GTPUSocket::close() {
    if (fd_ == INVALID_SOCKET_VAL) return;
#ifdef AUSF_PLATFORM_WINDOWS
    ::closesocket(fd_);
#else
    ::close(fd_);
#endif
    fd_ = INVALID_SOCKET_VAL;
}

bool GTPUSocket::recv(std::vector<uint8_t>& buf, PeerAddress& peer,
                      std::size_t max_size) {
    if (fd_ == INVALID_SOCKET_VAL) return false;
    buf.resize(max_size);

    struct sockaddr_in from{};
#ifdef AUSF_PLATFORM_WINDOWS
    int from_len = static_cast<int>(sizeof(from));
    const int n = ::recvfrom(fd_,
                             reinterpret_cast<char*>(buf.data()),
                             static_cast<int>(max_size), 0,
                             reinterpret_cast<struct sockaddr*>(&from),
                             &from_len);
    if (n == SOCKET_ERROR) { buf.clear(); return false; }
#else
    socklen_t from_len = sizeof(from);
    const ssize_t n = ::recvfrom(fd_, buf.data(), max_size, 0,
                                  reinterpret_cast<struct sockaddr*>(&from),
                                  &from_len);
    if (n <= 0) { buf.clear(); return false; }
#endif

    buf.resize(static_cast<std::size_t>(n));
    peer.ip   = from.sin_addr.s_addr;
    peer.port = ntohs(from.sin_port);
    return true;
}

bool GTPUSocket::send(const std::vector<uint8_t>& buf, const PeerAddress& peer) {
    if (fd_ == INVALID_SOCKET_VAL) return false;

    struct sockaddr_in dst{};
    dst.sin_family      = AF_INET;
    dst.sin_port        = htons(peer.port);
    dst.sin_addr.s_addr = peer.ip;

#ifdef AUSF_PLATFORM_WINDOWS
    const int n = ::sendto(fd_,
                           reinterpret_cast<const char*>(buf.data()),
                           static_cast<int>(buf.size()), 0,
                           reinterpret_cast<const struct sockaddr*>(&dst),
                           static_cast<int>(sizeof(dst)));
    return n == static_cast<int>(buf.size());
#else
    const ssize_t n = ::sendto(fd_, buf.data(), buf.size(), 0,
                                reinterpret_cast<const struct sockaddr*>(&dst),
                                sizeof(dst));
    return n == static_cast<ssize_t>(buf.size());
#endif
}

} // namespace networking
} // namespace ausf
