#pragma once

// UDP transport for PFCP — TS 29.244 §7.1
//
// PFCPSocket wraps a POSIX UDP socket bound to 0.0.0.0:<port>.
// It is intentionally kept separate from PFCPHandler so the handler logic
// can be unit-tested without a real socket (pass raw bytes directly via
// handleMessage / handleRawPDU).
//
// Platform: POSIX (Linux / macOS).  Windows support requires Winsock2 —
// compile with -DAUSF_PLATFORM_WINDOWS to use WSA instead of POSIX calls.

#include <cstdint>
#include <string>
#include <vector>

#ifdef AUSF_PLATFORM_WINDOWS
#  include <winsock2.h>
#  include <ws2tcpip.h>
   using socket_t = SOCKET;
   static constexpr socket_t INVALID_SOCKET_VAL = INVALID_SOCKET;
#else
#  include <arpa/inet.h>
#  include <netinet/in.h>
#  include <sys/socket.h>
#  include <unistd.h>
   using socket_t = int;
   static constexpr socket_t INVALID_SOCKET_VAL = -1;
#endif

namespace ausf {
namespace networking {

// MTU / fragmentation constants (TS 29.244 §7.1 — PFCP over UDP/IPv4).
//
// PFCP_ETH_MTU: practical send limit for standard Ethernet paths.
//   1500 (Ethernet payload) - 20 (IPv4 hdr) - 8 (UDP hdr) = 1472 bytes.
//   PDUs larger than this will cross an IP fragment boundary on untuned paths.
//
// PFCP_UDP_HARD_LIMIT: absolute maximum for an IPv4 UDP payload.
//   65535 (IPv4 total length max) - 20 (IPv4 hdr) - 8 (UDP hdr) = 65507 bytes.
//   PFCPSocket::send() rejects payloads exceeding this value unconditionally.
constexpr std::size_t PFCP_ETH_MTU       = 1472u;
constexpr std::size_t PFCP_UDP_HARD_LIMIT = 65507u;

// Peer address (IPv4) returned alongside each received datagram.
struct PeerAddress {
    uint32_t ip   = 0;   // network byte order
    uint16_t port = 0;
};

class PFCPSocket {
public:
    explicit PFCPSocket(int port);
    ~PFCPSocket();

    // Non-copyable, non-movable (owns OS resource).
    PFCPSocket(const PFCPSocket&)            = delete;
    PFCPSocket& operator=(const PFCPSocket&) = delete;

    // Open the socket and bind to 0.0.0.0:<port>.
    // Returns true on success, false + logs reason on failure.
    bool open();

    // Close the socket (idempotent; also called by destructor).
    void close();

    bool isOpen() const noexcept { return fd_ != INVALID_SOCKET_VAL; }

    // Block until a datagram arrives (up to max_size bytes) or the socket is
    // closed.  Returns false on error or if the socket was closed.
    // On success fills `buf` with the received bytes and `peer` with sender.
    bool recv(std::vector<uint8_t>& buf, PeerAddress& peer,
              std::size_t max_size = 65535);

    // Send `buf` to the specified peer.
    bool send(const std::vector<uint8_t>& buf, const PeerAddress& peer);

    int port() const noexcept { return port_; }

    // MTU hint for outbound PDUs (default: PFCP_ETH_MTU = 1472).
    // PDUs larger than this value are logged as warnings and sent anyway —
    // the IP layer is configured to allow fragmentation in open().
    // PDUs larger than PFCP_UDP_HARD_LIMIT (65507) are rejected in send().
    void        setMtu(std::size_t mtu) noexcept { mtu_ = mtu; }
    std::size_t mtu()             const noexcept { return mtu_; }

private:
    int         port_;
    socket_t    fd_  = INVALID_SOCKET_VAL;
    std::size_t mtu_ = PFCP_ETH_MTU;
};

} // namespace networking
} // namespace ausf
