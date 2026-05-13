#pragma once

// GTP-U header codec and socket — TS 29.281 §5.1
//
// GTP-U (GPRS Tunnelling Protocol for User Plane) carries user-plane traffic
// between gNB and UPF (N3 interface) and between UPF instances (N9 interface).
//
// This header provides:
//   GTPUMessageType  — message type enum (TS 29.281 Table 7.1-1, subset)
//   GTPUHeader       — parsed header fields (mandatory 8 bytes + optional 4 bytes)
//   encodeGTPUHeader — serialize header to wire bytes
//   decodeGTPUHeader — parse header from wire bytes
//   encodeGTPU       — build complete frame (header + payload, auto-sets length)
//   GTPUFrame        — decoded frame (header + payload)
//   decodeGTPU       — split raw frame into header + payload
//   GTPUSocket       — cross-platform UDP socket bound to GTP-U port 2152

#include "pfcp_socket.h"   // for PeerAddress, socket_t, INVALID_SOCKET_VAL

#include <cstdint>
#include <optional>
#include <vector>

namespace ausf {
namespace networking {

// TS 29.281 Table 7.1-1 — selected message types
enum class GTPUMessageType : uint8_t {
    ECHO_REQUEST                             = 0x01,
    ECHO_RESPONSE                            = 0x02,
    ERROR_INDICATION                         = 0x1A,
    SUPPORTED_EXT_HEADERS_NOTIFICATION       = 0x1F,
    END_MARKER                               = 0xFE,
    G_PDU                                    = 0xFF,  // encapsulated IP packet
};

// GTP-U PDU header — TS 29.281 §5.1
//
// Mandatory part (always 8 bytes):
//   Oct 1:  [Ver(3)=1 | PT=1 | spare | E | S | PN]
//   Oct 2:  Message Type
//   Oct 3-4: Length (bytes after this 8-byte header; big-endian)
//   Oct 5-8: TEID (big-endian)
//
// Optional part (present when E || S || PN; adds 4 bytes):
//   Oct 9-10: Sequence Number (big-endian)
//   Oct 11:   N-PDU Number
//   Oct 12:   Next Extension Header Type (0 = none)
struct GTPUHeader {
    uint8_t         version         = 1;     // always 1 for GTP-U
    bool            pt              = true;  // Protocol Type bit; always 1
    bool            e               = false; // Extension Header Flag
    bool            s               = false; // Sequence Number Flag
    bool            pn              = false; // N-PDU Number Flag
    GTPUMessageType message_type    = GTPUMessageType::G_PDU;
    uint16_t        length          = 0;     // bytes after mandatory 8-byte header
    uint32_t        teid            = 0;
    // Optional fields (meaningful when e || s || pn)
    uint16_t        sequence_number = 0;
    uint8_t         npdu_number     = 0;
    uint8_t         next_ext_hdr    = 0;    // 0 = no extension header

    // Wire size of this header: 8 bytes mandatory, +4 when optional fields present.
    std::size_t wireSize() const noexcept {
        return (e || s || pn) ? 12u : 8u;
    }
};

// Encode a GTP-U header to wire bytes.
std::vector<uint8_t> encodeGTPUHeader(const GTPUHeader& hdr);

// Decode a GTP-U header from the start of a byte buffer.
// Returns std::nullopt when:
//   - buffer size < 8
//   - version != 1
//   - optional fields present (E|S|PN) but buffer size < 12
std::optional<GTPUHeader> decodeGTPUHeader(const std::vector<uint8_t>& buf);

// Build a complete GTP-U frame: header + payload.
// Sets hdr.length automatically: optional-field-size (4 if E|S|PN else 0) + payload.size().
std::vector<uint8_t> encodeGTPU(GTPUHeader hdr, const std::vector<uint8_t>& payload);

// ---------------------------------------------------------------------------
// PDU Session Container extension header — TS 38.415 §5.5
//
// Carried as a GTP-U extension header (Next Extension Header Type = 0x85).
// Wire format (4 bytes total, Extension Header Length = 0x01):
//   Octet 1: 0x01  (length = 1 × 4 bytes)
//   Octet 2: [PDU Type(4) | spare(4)]  — PDU Type: 0=DL, 1=UL
//   Octet 3: [spare(2) | QFI(6)]
//   Octet 4: 0x00  (no next extension header)
// ---------------------------------------------------------------------------
struct PduSessionContainer {
    bool    uplink = true;  // true = UL PDU Session Container (type=1)
    uint8_t qfi    = 0;     // QoS Flow Identifier (6 bits, TS 38.415 §5.5.1)
};

// Next Extension Header Type value for PDU Session Container (TS 38.415).
constexpr uint8_t GTPU_EXT_HDR_PDU_SESSION_CONTAINER = 0x85u;

// A fully decoded GTP-U frame.
struct GTPUFrame {
    GTPUHeader           header;
    std::vector<uint8_t> payload;  // bytes after header (and any ext hdrs)
    // Populated by decodeGTPU when a PDU Session Container ext hdr is present.
    std::optional<PduSessionContainer> pdu_session_container;
};

// Split a raw GTP-U frame into header + optional PDU Session Container + payload.
// Returns std::nullopt if the frame is malformed.
std::optional<GTPUFrame> decodeGTPU(const std::vector<uint8_t>& frame);

// Build a complete GTP-U frame carrying a PDU Session Container extension header.
// Sets E=true, next_ext_hdr=0x85, and adjusts hdr.length automatically.
std::vector<uint8_t> encodeGTPU(GTPUHeader hdr, const std::vector<uint8_t>& payload,
                                 const PduSessionContainer& psc);

// ---------------------------------------------------------------------------
// GTPUSocket — UDP socket bound to port 2152 (GTP-U well-known port)
//
// Cross-platform: Windows (Winsock2) when compiled with -DAUSF_PLATFORM_WINDOWS,
// POSIX otherwise.
// ---------------------------------------------------------------------------
class GTPUSocket {
public:
    explicit GTPUSocket(uint16_t port = 2152);
    ~GTPUSocket();

    GTPUSocket(const GTPUSocket&)            = delete;
    GTPUSocket& operator=(const GTPUSocket&) = delete;

    // Open the socket and bind to 0.0.0.0:<port>.
    bool open();

    // Close the socket (idempotent).
    void close();

    bool isOpen() const noexcept { return fd_ != INVALID_SOCKET_VAL; }

    // Block until a datagram arrives (up to max_size bytes) or socket is closed.
    bool recv(std::vector<uint8_t>& buf, PeerAddress& peer,
              std::size_t max_size = 65535);

    // Send a datagram to peer.
    bool send(const std::vector<uint8_t>& buf, const PeerAddress& peer);

private:
    uint16_t port_;
    socket_t fd_ = INVALID_SOCKET_VAL;
};

} // namespace networking
} // namespace ausf
