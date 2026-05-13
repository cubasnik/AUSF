#pragma once

// Data-plane forwarder — TS 29.244 §6 / TS 29.281
//
// DataPlane receives decoded GTP-U frames from the access-side GTP-U socket,
// looks up the matching Packet Detection Rule (PDR) by TEID, applies the
// corresponding Forwarding Action Rule (FAR), and accounts traffic against
// the linked Usage Reporting Rule (URR).
//
// This class is deliberately I/O-free: it operates on GTPUFrame objects and
// returns encoded byte vectors that the caller can send via GTPUSocket.
// This makes it straightforward to unit-test without network fixtures.

#include "gtpu.h"
#include "pfcp_handler.h"

#include <cstdint>
#include <memory>
#include <mutex>
#include <unordered_map>
#include <vector>

namespace ausf {
namespace networking {

// Result of processing one incoming GTP-U frame.
struct ForwardResult {
    bool     forwarded    = false;  // FAR said FORW and frame is encoded in `frame`
    bool     dropped      = false;  // FAR said DROP (packet discarded)
    bool     gated        = false;  // QER gate was CLOSED (packet dropped by gating)
    bool     buffered     = false;  // FAR said BUFF (packet stored in buffer)
    std::vector<uint8_t> frame;     // encoded outgoing GTP-U frame (meaningful when forwarded)
    uint64_t session_seid = 0;      // SEID of the session that processed this packet (0 = no match)
    uint32_t pdr_id       = 0;      // PDR that matched (0 = no match)
    uint32_t far_id       = 0;      // FAR that was applied (0 = no match)
};

// DataPlane: rule-based GTP-U packet processing engine.
//
// Thread-safety: addSession / removeSession / processUplink may be called
// from different threads.  All public methods are protected by a mutex.
class DataPlane {
public:
    // Register a session with this data plane.
    // Indexes every PDR whose PDI carries a local F-TEID for uplink lookup.
    void addSession(std::shared_ptr<PFCPSession> session);

    // Deregister a session and remove its TEID index entries.
    void removeSession(uint64_t seid);

    // Process an incoming GTP-U uplink frame (access → core direction).
    //
    // Procedure:
    //   1. Look up PDR by frame.header.teid.
    //   2. Resolve the FAR referenced by the PDR.
    //   3. Apply QER gate check (if PDR references a QER with UL gate CLOSED → gated drop).
    //   4. Apply FAR action:
    //        FORW → build outgoing GTP-U frame (using FAR's outer-header-creation TEID if set,
    //               otherwise preserve the original TEID); account octets.
    //        DROP → account octets, mark result.dropped.
    //        BUFF → store payload in session buffer, mark result.buffered.
    //   5. Account traffic against the PFCPSession's UL counters.
    ForwardResult processUplink(const GTPUFrame& frame);

    // Drain all buffered payloads for a session (BUFF'd packets).
    // Returns the buffered IP payloads and clears the buffer.
    std::vector<std::vector<uint8_t>> drainBuffer(uint64_t seid);

    // Number of sessions currently registered.
    std::size_t sessionCount() const;

private:
    mutable std::mutex mutex_;

    // TEID of PDI.local_fteid → session (uplink lookup key)
    std::unordered_map<uint32_t, std::shared_ptr<PFCPSession>> teid_index_;

    // SEID → session (needed for removeSession to un-index TEIDs)
    std::unordered_map<uint64_t, std::shared_ptr<PFCPSession>> seid_index_;

    // Buffered packets per session SEID (BUFF action).
    std::unordered_map<uint64_t, std::vector<std::vector<uint8_t>>> buffer_;

    // Returns the PDR with the lowest precedence that matches `teid`.
    // Caller must hold mutex_.
    const PDRRule* matchPDR(const PFCPSession& session, uint32_t teid) const;

    // Returns the FAR with the given far_id, or nullptr.
    // Caller must hold mutex_.
    const FARRule* findFAR(const PFCPSession& session, uint32_t far_id) const;
};

} // namespace networking
} // namespace ausf
