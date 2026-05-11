#pragma once

#include "authentication_result.h"
#include "pfcp_ie.h"
#include "pfcp_rules.h"
#include "pfcp_socket.h"

#include <atomic>
#include <chrono>
#include <cstdint>
#include <memory>
#include <optional>
#include <string>
#include <thread>
#include <unordered_map>
#include <vector>

namespace ausf {
namespace networking {

enum class PFCPMessageType : uint8_t {
    HEARTBEAT_REQUEST              = 1,
    HEARTBEAT_RESPONSE             = 2,
    ASSOCIATION_SETUP_REQUEST      = 5,
    ASSOCIATION_SETUP_RESPONSE     = 6,
    SESSION_ESTABLISHMENT_REQUEST  = 50,
    SESSION_ESTABLISHMENT_RESPONSE = 51,
    SESSION_MODIFICATION_REQUEST   = 52,
    SESSION_MODIFICATION_RESPONSE  = 53,
    SESSION_DELETION_REQUEST       = 54,
    SESSION_DELETION_RESPONSE      = 55,
};

// Represents one active PFCP session bound to an authenticated subscriber.
//
// Sessions are created only after the handler validates that the SUPI carried
// in the SESSION_ESTABLISHMENT_REQUEST has a confirmed AUTHENTICATED result,
// preventing data-plane traffic for unauthenticated UEs.
class PFCPSession {
public:
    PFCPSession(uint64_t seid, std::string supi, AuthenticationResult auth_result);
    ~PFCPSession();

    void handleSessionEstablishment();
    void handleSessionModification();
    void handleSessionDeletion();

    // Install PDR/FAR/URR/QER rules from a SESSION_ESTABLISHMENT or MODIFICATION request.
    void installRules(std::vector<PDRRule>  pdrs,
                      std::vector<FARRule>  fars,
                      std::vector<URRRule>  urrs,
                      std::vector<QERRule>  qers = {});

    // Merge updated rules into the session (used by SESSION_MODIFICATION).
    // For each rule in the updated vectors: replaces an existing rule with the
    // same ID, or appends a new rule if no matching ID is found.
    void updateRules(std::vector<PDRRule>  updated_pdrs,
                     std::vector<FARRule>  updated_fars,
                     std::vector<URRRule>  updated_urrs = {},
                     std::vector<QERRule>  updated_qers = {});

    // Account for user-plane traffic (called by data-plane forwarder).
    void accountUplinkOctets  (uint64_t octets, uint64_t packets = 1) noexcept;
    void accountDownlinkOctets(uint64_t octets, uint64_t packets = 1) noexcept;

    // Generate usage reports for all installed URRs.
    //   periodic=true  → only include URRs with PERIO trigger
    //   periodic=false → include all (used on session deletion)
    std::vector<UsageReport> collectUsageReports(bool periodic);

    // Check whether any VOLTH-triggered URR has had its volume threshold crossed
    // since the last check.  Returns triggered reports (empty if none crossed).
    // Should be called after accountUplinkOctets / accountDownlinkOctets.
    std::vector<UsageReport> checkVolumeThresholds();

    // Periodic usage reporting — TS 29.244 §8.2.41 (PERIO trigger).
    // Checks each URR whose reporting_triggers.perio == true and whose
    // measurement_period has elapsed since the last collection.
    // On first call the per-URR timer is initialised to 'now' (no immediate fire).
    std::vector<UsageReport> tickPeriodicReports(
        std::chrono::steady_clock::time_point now);

    uint64_t getSEID()          const noexcept { return seid_; }
    const std::string& getSupi() const noexcept { return supi_; }
    const std::string& getStatus() const noexcept { return status_; }
    const AuthenticationResult& getAuthResult() const noexcept { return auth_result_; }

    uint64_t getUplinkOctets()    const noexcept { return ul_octets_.load(std::memory_order_relaxed); }
    uint64_t getDownlinkOctets()  const noexcept { return dl_octets_.load(std::memory_order_relaxed); }
    uint64_t getUplinkPackets()   const noexcept { return ul_packets_.load(std::memory_order_relaxed); }
    uint64_t getDownlinkPackets() const noexcept { return dl_packets_.load(std::memory_order_relaxed); }

    const std::vector<PDRRule>& getPDRs() const noexcept { return pdrs_; }
    const std::vector<FARRule>& getFARs() const noexcept { return fars_; }
    const std::vector<URRRule>& getURRs() const noexcept { return urrs_; }
    const std::vector<QERRule>& getQERs() const noexcept { return qers_; }

    // Look up a QER by ID.  Returns nullptr if not found.
    const QERRule* findQER(uint32_t qer_id) const noexcept;

private:
    uint64_t             seid_;
    std::string          supi_;
    std::string          status_;
    AuthenticationResult auth_result_;

    std::vector<PDRRule>  pdrs_;
    std::vector<FARRule>  fars_;
    std::vector<URRRule>  urrs_;
    std::vector<QERRule>  qers_;

    std::atomic<uint64_t> ul_octets_{0};
    std::atomic<uint64_t> dl_octets_{0};
    std::atomic<uint64_t> ul_packets_{0};
    std::atomic<uint64_t> dl_packets_{0};

    // Per-URR usage report sequence numbers (incremented on each collection).
    std::unordered_map<uint32_t, uint32_t> ur_seqn_;

    // Tracks how many times each URR's volume threshold has already triggered
    // (= floor(total_octets / threshold) at last report).
    std::unordered_map<uint32_t, uint32_t> volth_crossed_;

    // Per-URR timestamp of last periodic collection (used by tickPeriodicReports).
    std::unordered_map<uint32_t, std::chrono::steady_clock::time_point> perio_last_fired_;
};

// Manages PFCP association and sessions, integrated with AUSF authentication results.
//
// Lifecycle:
//   1. initialize() → start()
//   2. notifyAuthenticationResult() — called when authentication completes
//   3. handleMessage()              — processes incoming PFCP PDUs
//   4. stop()
//
// Session establishment gating (TS 29.244):
//   A SESSION_ESTABLISHMENT_REQUEST must carry an AUSF_SUPI IE (enterprise
//   extension 0x8001).  The handler looks up the SUPI in the authentication
//   registry and rejects the request unless status == AUTHENTICATED.
class PFCPHandler {
public:
    explicit PFCPHandler(int port);
    ~PFCPHandler();

    bool initialize();
    bool start();
    void stop();

    // Called by the authentication service when a UE authentication procedure
    // completes (success or rejection).  Replaces any prior result for the SUPI.
    void notifyAuthenticationResult(AuthenticationResult result);

    // Look up the stored authentication result for a SUPI.
    // Returns std::nullopt if no result has been notified.
    std::optional<AuthenticationResult> getAuthResult(const std::string& supi) const;

    // Process a raw PFCP PDU (wire bytes including header).  Can be called
    // directly in tests without a real socket.
    bool handleMessage(const std::vector<uint8_t>& raw_pdu);

    bool        isAssociationEstablished() const noexcept { return association_established_; }
    std::size_t getSessionCount()          const noexcept { return sessions_.size(); }

    // Look up a session by SEID.  Returns nullptr if not found.
    std::shared_ptr<PFCPSession> getSession(uint64_t seid) const {
        auto it = sessions_.find(seid);
        return it != sessions_.end() ? it->second : nullptr;
    }

    // Manually trigger a periodic-report tick for all sessions (also called by
    // the internal timer thread every ~200 ms).  Exposed for testing.
    std::vector<UsageReport> tickAllPeriodicReports();

private:
    int  port_;
    std::atomic<bool> is_running_{false};
    bool association_established_ = false;

    std::unique_ptr<PFCPSocket> socket_;
    std::thread                 recv_thread_;
    std::thread                 timer_thread_;
    std::atomic<bool>           timer_stop_{false};

    std::unordered_map<uint64_t, std::shared_ptr<PFCPSession>> sessions_;

    // Keyed by SUPI; updated by notifyAuthenticationResult().
    std::unordered_map<std::string, AuthenticationResult> auth_registry_;

    // Receive loop — runs in recv_thread_.
    void recvLoop();

    // Periodic-report timer loop — runs in timer_thread_.
    void timerLoop();

    // Build a minimal PFCP response with given IE payload.
    std::vector<uint8_t> buildResponse(uint8_t msg_type,
                                       bool seid_present,
                                       uint64_t seid,
                                       uint32_t seq,
                                       const std::vector<uint8_t>& ie_payload) const;

    // Auth-gated session establishment — extracts AUSF_SUPI IE and validates registry.
    bool handleSessionEstablishment(const PFCPHeader& header,
                                    const std::vector<uint8_t>& payload);

    static std::string messageTypeToString(uint8_t message_type);
};

} // namespace networking
} // namespace ausf
