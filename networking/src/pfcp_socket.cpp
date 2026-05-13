#include "pfcp_socket.h"

#include <cstdio>
#include <cstring>

#ifdef AUSF_PLATFORM_WINDOWS
#  include <ws2tcpip.h>   // IP_DONTFRAGMENT
#else
#  include <netinet/ip.h>  // IP_PMTUDISC_DONT
#endif

#ifdef AUSF_PLATFORM_WINDOWS
#  pragma comment(lib, "ws2_32.lib")
#endif

namespace ausf {
namespace networking {

PFCPSocket::PFCPSocket(int port) : port_(port) {
#ifdef AUSF_PLATFORM_WINDOWS
    WSADATA wsa{};
    WSAStartup(MAKEWORD(2, 2), &wsa);
#endif
}

PFCPSocket::~PFCPSocket() {
    close();
#ifdef AUSF_PLATFORM_WINDOWS
    WSACleanup();
#endif
}

bool PFCPSocket::open() {
    fd_ = ::socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (fd_ == INVALID_SOCKET_VAL) {
        std::perror("pfcp_socket: socket()");
        return false;
    }

    // Allow re-use of the port if the process is restarted quickly.
    int optval = 1;
#ifdef AUSF_PLATFORM_WINDOWS
    ::setsockopt(fd_, SOL_SOCKET, SO_REUSEADDR,
                 reinterpret_cast<const char*>(&optval), sizeof(optval));
#else
    ::setsockopt(fd_, SOL_SOCKET, SO_REUSEADDR, &optval, sizeof(optval));
#endif

    // Explicitly allow IP-layer fragmentation for large PDUs.
    // PFCP sessions with many IEs can exceed a single Ethernet MTU; we rely on
    // the IP layer to reassemble rather than silently dropping oversized sends.
#ifdef AUSF_PLATFORM_WINDOWS
    {
        int dontfrag = 0; // 0 = allow fragmentation
        ::setsockopt(fd_, IPPROTO_IP, IP_DONTFRAGMENT,
                     reinterpret_cast<const char*>(&dontfrag), sizeof(dontfrag));
    }
#else
    {
        int pmtud = IP_PMTUDISC_DONT; // disable PMTU discovery → allow fragmentation
        ::setsockopt(fd_, IPPROTO_IP, IP_MTU_DISCOVER, &pmtud, sizeof(pmtud));
    }
#endif

    struct sockaddr_in addr{};
    addr.sin_family      = AF_INET;
    addr.sin_port        = htons(static_cast<uint16_t>(port_));
    addr.sin_addr.s_addr = htonl(INADDR_ANY);

    if (::bind(fd_, reinterpret_cast<struct sockaddr*>(&addr), sizeof(addr)) != 0) {
        std::perror("pfcp_socket: bind()");
        close();
        return false;
    }
    return true;
}

void PFCPSocket::close() {
    if (fd_ == INVALID_SOCKET_VAL) return;
#ifdef AUSF_PLATFORM_WINDOWS
    ::closesocket(fd_);
#else
    ::close(fd_);
#endif
    fd_ = INVALID_SOCKET_VAL;
}

bool PFCPSocket::recv(std::vector<uint8_t>& buf, PeerAddress& peer,
                      std::size_t max_size) {
    if (fd_ == INVALID_SOCKET_VAL) return false;

    buf.resize(max_size);

    struct sockaddr_in from{};
#ifdef AUSF_PLATFORM_WINDOWS
    int from_len = sizeof(from);
    const int n = ::recvfrom(fd_,
                             reinterpret_cast<char*>(buf.data()),
                             static_cast<int>(max_size), 0,
                             reinterpret_cast<struct sockaddr*>(&from),
                             &from_len);
    if (n == SOCKET_ERROR) { buf.clear(); return false; }
#else
    socklen_t from_len = sizeof(from);
    const ssize_t n = ::recvfrom(fd_,
                                  buf.data(), max_size, 0,
                                  reinterpret_cast<struct sockaddr*>(&from),
                                  &from_len);
    if (n < 0) { buf.clear(); return false; }
#endif

    buf.resize(static_cast<std::size_t>(n));
    peer.ip   = ntohl(from.sin_addr.s_addr);
    peer.port = ntohs(from.sin_port);
    return true;
}

bool PFCPSocket::send(const std::vector<uint8_t>& buf, const PeerAddress& peer) {
    if (fd_ == INVALID_SOCKET_VAL) return false;

    // Hard limit: IPv4 UDP payload cannot exceed 65507 bytes.
    if (buf.size() > PFCP_UDP_HARD_LIMIT) {
        std::fprintf(stderr,
                     "[PFCP] send: PDU size %zu exceeds IPv4 UDP hard limit %zu — rejected\n",
                     buf.size(), PFCP_UDP_HARD_LIMIT);
        return false;
    }

    // Soft limit: PDUs larger than the configured MTU hint will be
    // IP-fragmented by the OS.  Log a diagnostic to aid troubleshooting.
    if (buf.size() > mtu_) {
        std::fprintf(stderr,
                     "[PFCP] send: PDU size %zu > MTU %zu — IP fragmentation required\n",
                     buf.size(), mtu_);
        // Fall through: IP_DONTFRAGMENT=0 set in open() allows the OS to fragment.
    }

    struct sockaddr_in to{};
    to.sin_family      = AF_INET;
    to.sin_port        = htons(peer.port);
    to.sin_addr.s_addr = htonl(peer.ip);

#ifdef AUSF_PLATFORM_WINDOWS
    const int n = ::sendto(fd_,
                           reinterpret_cast<const char*>(buf.data()),
                           static_cast<int>(buf.size()), 0,
                           reinterpret_cast<struct sockaddr*>(&to),
                           sizeof(to));
    return n != SOCKET_ERROR;
#else
    const ssize_t n = ::sendto(fd_, buf.data(), buf.size(), 0,
                                reinterpret_cast<struct sockaddr*>(&to),
                                sizeof(to));
    return n >= 0;
#endif
}

} // namespace networking
} // namespace ausf
