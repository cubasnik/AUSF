#include "pfcp_handler.h"
#include <iostream>

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// PFCPSession
// ---------------------------------------------------------------------------

PFCPSession::PFCPSession(uint64_t seid, std::string supi, AuthenticationResult auth_result)
    : seid_(seid)
    , supi_(std::move(supi))
    , status_("CREATED")
    , auth_result_(std::move(auth_result)) {
    std::cout << "[PFCP] Session created seid=" << seid_
              << " supi=" << supi_
              << " auth=" << auth_result_.statusString() << std::endl;
}

PFCPSession::~PFCPSession() {
    std::cout << "[PFCP] Session destroyed seid=" << seid_
              << " supi=" << supi_ << std::endl;
}

void PFCPSession::handleSessionEstablishment() {
    status_ = "SESSION_ESTABLISHED";
    std::cout << "[PFCP] Session established seid=" << seid_
              << " supi=" << supi_ << std::endl;
}

void PFCPSession::handleSessionModification() {
    status_ = "SESSION_MODIFIED";
    std::cout << "[PFCP] Session modified seid=" << seid_
              << " supi=" << supi_ << std::endl;
}

void PFCPSession::handleSessionDeletion() {
    status_ = "SESSION_DELETED";
    std::cout << "[PFCP] Session deleted seid=" << seid_
              << " supi=" << supi_ << std::endl;
}

void PFCPSession::installRules(std::vector<PDRRule>  pdrs,
                                std::vector<FARRule>  fars,
                                std::vector<URRRule>  urrs,
                                std::vector<QERRule>  qers) {
    pdrs_ = std::move(pdrs);
    fars_ = std::move(fars);
    urrs_ = std::move(urrs);
    qers_ = std::move(qers);
    std::cout << "[PFCP] Session seid=" << seid_ << " rules installed:"
              << " PDR=" << pdrs_.size()
              << " FAR=" << fars_.size()
              << " URR=" << urrs_.size()
              << " QER=" << qers_.size() << std::endl;
}

void PFCPSession::updateRules(std::vector<PDRRule>  updated_pdrs,
                               std::vector<FARRule>  updated_fars,
                               std::vector<URRRule>  updated_urrs,
                               std::vector<QERRule>  updated_qers) {
    // For each updated rule: replace existing rule with same ID, or append if new.
    for (auto& upd : updated_pdrs) {
        bool found = false;
        for (auto& ex : pdrs_) {
            if (ex.pdr_id == upd.pdr_id) { ex = std::move(upd); found = true; break; }
        }
        if (!found) pdrs_.push_back(std::move(upd));
    }
    for (auto& upd : updated_fars) {
        bool found = false;
        for (auto& ex : fars_) {
            if (ex.far_id == upd.far_id) { ex = std::move(upd); found = true; break; }
        }
        if (!found) fars_.push_back(std::move(upd));
    }
    for (auto& upd : updated_urrs) {
        bool found = false;
        for (auto& ex : urrs_) {
            if (ex.urr_id == upd.urr_id) { ex = std::move(upd); found = true; break; }
        }
        if (!found) urrs_.push_back(std::move(upd));
    }
    for (auto& upd : updated_qers) {
        bool found = false;
        for (auto& ex : qers_) {
            if (ex.qer_id == upd.qer_id) { ex = std::move(upd); found = true; break; }
        }
        if (!found) qers_.push_back(std::move(upd));
    }
    std::cout << "[PFCP] Session seid=" << seid_ << " rules updated:"
              << " PDR=" << pdrs_.size()
              << " FAR=" << fars_.size()
              << " URR=" << urrs_.size()
              << " QER=" << qers_.size() << std::endl;
}

const QERRule* PFCPSession::findQER(uint32_t qer_id) const noexcept {
    for (const auto& q : qers_) {
        if (q.qer_id == qer_id) return &q;
    }
    return nullptr;
}

void PFCPSession::accountUplinkOctets(uint64_t octets, uint64_t packets) noexcept {
    ul_octets_.fetch_add(octets,  std::memory_order_relaxed);
    ul_packets_.fetch_add(packets, std::memory_order_relaxed);
}

void PFCPSession::accountDownlinkOctets(uint64_t octets, uint64_t packets) noexcept {
    dl_octets_.fetch_add(octets,  std::memory_order_relaxed);
    dl_packets_.fetch_add(packets, std::memory_order_relaxed);
}

std::vector<UsageReport> PFCPSession::collectUsageReports(bool periodic) {
    std::vector<UsageReport> reports;
    const uint64_t ul = ul_octets_.load(std::memory_order_relaxed);
    const uint64_t dl = dl_octets_.load(std::memory_order_relaxed);

    for (const auto& urr : urrs_) {
        if (periodic && !urr.reporting_triggers.perio) continue;
        UsageReport rep;
        rep.urr_id  = urr.urr_id;
        rep.ur_seqn = ++ur_seqn_[urr.urr_id];
        rep.trigger.perio = periodic;
        rep.volume  = VolumeMeasurement::withULDL(ul, dl);
        reports.push_back(rep);
    }
    return reports;
}

std::vector<UsageReport> PFCPSession::checkVolumeThresholds() {
    std::vector<UsageReport> reports;
    const uint64_t ul    = ul_octets_.load(std::memory_order_relaxed);
    const uint64_t dl    = dl_octets_.load(std::memory_order_relaxed);
    const uint64_t total = ul + dl;

    for (const auto& urr : urrs_) {
        if (!urr.reporting_triggers.volth) continue;
        if (!urr.volume_threshold.has_value()) continue;

        // Compute effective threshold as the TOTAL value if present,
        // otherwise sum of ul+dl threshold fields.
        uint64_t thresh = 0;
        const auto& vt = *urr.volume_threshold;
        if (vt.total_present && vt.total_octets > 0) {
            thresh = vt.total_octets;
        } else if (vt.ul_present || vt.dl_present) {
            thresh = vt.ul_octets + vt.dl_octets;
        }
        if (thresh == 0) continue;

        // How many times has the threshold been crossed by total traffic?
        const uint32_t crossings = static_cast<uint32_t>(total / thresh);
        uint32_t& last = volth_crossed_[urr.urr_id]; // default 0
        if (crossings > last) {
            last = crossings;
            UsageReport rep;
            rep.urr_id  = urr.urr_id;
            rep.ur_seqn = ++ur_seqn_[urr.urr_id];
            rep.trigger.volth = true;
            rep.volume  = VolumeMeasurement::withULDL(ul, dl);
            reports.push_back(rep);
            std::cout << "[PFCP] seid=" << seid_
                      << " URR " << urr.urr_id
                      << " VOLTH crossed " << crossings << "x"
                      << " total=" << total << " thresh=" << thresh << std::endl;
        }
    }
    return reports;
}

std::vector<UsageReport> PFCPSession::tickPeriodicReports(
        std::chrono::steady_clock::time_point now) {
    std::vector<UsageReport> reports;
    for (auto& urr : urrs_) {
        if (!urr.reporting_triggers.perio) continue;
        if (urr.measurement_period == 0u) continue;

        auto& last = perio_last_fired_[urr.urr_id];
        // Initialise on first tick — start the clock, don't fire immediately.
        if (last == std::chrono::steady_clock::time_point{}) {
            last = now;
            continue;
        }

        const auto period = std::chrono::seconds(urr.measurement_period);
        if (now - last >= period) {
            last += period;  // advance by exactly one period (avoids drift)
            UsageReport rep;
            rep.urr_id  = urr.urr_id;
            rep.ur_seqn = ++ur_seqn_[urr.urr_id];
            rep.trigger = urr.reporting_triggers;
            rep.volume  = VolumeMeasurement::withULDL(
                ul_octets_.load(std::memory_order_relaxed),
                dl_octets_.load(std::memory_order_relaxed));
            reports.push_back(rep);
            std::cout << "[PFCP] seid=" << seid_
                      << " URR " << urr.urr_id
                      << " PERIO report seqn=" << rep.ur_seqn << std::endl;
        }
    }
    return reports;
}

// ---------------------------------------------------------------------------
// PFCPHandler
// ---------------------------------------------------------------------------

PFCPHandler::PFCPHandler(int port)
    : port_(port), association_established_(false) {
    std::cout << "[PFCP] Handler initialized port=" << port_ << std::endl;
}

PFCPHandler::~PFCPHandler() {
    if (is_running_) stop();
}

bool PFCPHandler::initialize() {
    std::cout << "[PFCP] Initializing port=" << port_ << std::endl;
    socket_ = std::make_unique<PFCPSocket>(port_);
    if (!socket_->open()) {
        std::cerr << "[PFCP] Failed to open socket" << std::endl;
        return false;
    }
    return true;
}

bool PFCPHandler::start() {
    if (!socket_ || !socket_->isOpen()) {
        std::cerr << "[PFCP] Call initialize() before start()" << std::endl;
        return false;
    }
    is_running_  = true;
    timer_stop_  = false;
    recv_thread_ = std::thread([this] { recvLoop(); });
    timer_thread_ = std::thread([this] { timerLoop(); });
    std::cout << "[PFCP] Handler started" << std::endl;
    return true;
}

void PFCPHandler::stop() {
    is_running_ = false;
    timer_stop_ = true;
    if (socket_) socket_->close(); // unblocks recvfrom
    if (recv_thread_.joinable())   recv_thread_.join();
    if (timer_thread_.joinable())  timer_thread_.join();
    std::cout << "[PFCP] Handler stopped" << std::endl;
}

void PFCPHandler::recvLoop() {
    std::vector<uint8_t> buf;
    PeerAddress peer;
    while (is_running_) {
        if (!socket_->recv(buf, peer)) break; // socket closed or error
        const bool ok = handleMessage(buf);
        if (!ok) {
            std::cerr << "[PFCP] handleMessage returned false for PDU from "
                      << peer.ip << ":" << peer.port << std::endl;
        }
    }
}

void PFCPHandler::timerLoop() {
    using namespace std::chrono;
    while (!timer_stop_.load(std::memory_order_relaxed)) {
        // Sleep in short slices so we react quickly to timer_stop_.
        for (int i = 0; i < 20 && !timer_stop_.load(std::memory_order_relaxed); ++i) {
            std::this_thread::sleep_for(milliseconds(10));
        }
        tickAllPeriodicReports();
    }
}

std::vector<UsageReport> PFCPHandler::tickAllPeriodicReports() {
    const auto now = std::chrono::steady_clock::now();
    std::vector<UsageReport> all;
    for (auto& kv : sessions_) {
        auto reports = kv.second->tickPeriodicReports(now);
        all.insert(all.end(), reports.begin(), reports.end());
    }
    return all;
}

void PFCPHandler::notifyAuthenticationResult(AuthenticationResult result) {
    std::cout << "[PFCP] Auth result notified supi=" << result.supi
              << " status=" << result.statusString()
              << " method=" << result.auth_method << std::endl;
    auth_registry_[result.supi] = std::move(result);
}

std::optional<AuthenticationResult> PFCPHandler::getAuthResult(const std::string& supi) const {
    auto it = auth_registry_.find(supi);
    if (it == auth_registry_.end()) return std::nullopt;
    return it->second;
}

bool PFCPHandler::handleMessage(const std::vector<uint8_t>& raw_pdu) {
    // Decode header from wire bytes.
    auto hdr_opt = decodePFCPHeader(raw_pdu);
    if (!hdr_opt) {
        std::cerr << "[PFCP] Failed to decode PFCP header (buf too short or bad version)"
                  << std::endl;
        return false;
    }
    const PFCPHeader& header = *hdr_opt;

    // Payload starts after the header.
    const std::size_t header_size = header.wireSize();
    const std::vector<uint8_t> payload(raw_pdu.begin() + header_size, raw_pdu.end());

    std::cout << "[PFCP] Message seq=" << header.sequence_number
              << " type=" << messageTypeToString(header.message_type)
              << " seid=" << header.seid << std::endl;

    switch (static_cast<PFCPMessageType>(header.message_type)) {

        case PFCPMessageType::HEARTBEAT_REQUEST:
            if (!association_established_) {
                std::cerr << "[PFCP] Reject HEARTBEAT: association not established" << std::endl;
                return false;
            }
            std::cout << "[PFCP] Heartbeat received — responding HEARTBEAT_RESPONSE" << std::endl;
            return true;

        case PFCPMessageType::ASSOCIATION_SETUP_REQUEST:
            association_established_ = true;
            std::cout << "[PFCP] Association established with peer node" << std::endl;
            return true;

        case PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST:
            return handleSessionEstablishment(header, payload);

        case PFCPMessageType::SESSION_MODIFICATION_REQUEST: {
            auto it = sessions_.find(header.seid);
            if (it == sessions_.end()) {
                std::cerr << "[PFCP] Reject SESSION_MODIFICATION: unknown seid="
                          << header.seid << std::endl;
                return false;
            }
            auto& session = it->second;
            const auto mod_ies = parseIEs(payload);

            std::vector<PDRRule> upd_pdrs;
            std::vector<FARRule> upd_fars;
            std::vector<URRRule> upd_urrs;
            std::vector<QERRule> upd_qers;

            for (const auto& ie : mod_ies) {
                if (ie.type == IEType::UPDATE_PDR || ie.type == IEType::CREATE_PDR) {
                    const auto inner = parseIEs(ie.value);
                    auto pdr = PDRRule::fromIEs(inner);
                    if (pdr) upd_pdrs.push_back(std::move(*pdr));
                } else if (ie.type == IEType::UPDATE_FAR || ie.type == IEType::CREATE_FAR) {
                    const auto inner = parseIEs(ie.value);
                    auto fr = FARRule::fromIEs(inner);
                    if (fr) upd_fars.push_back(std::move(*fr));
                } else if (ie.type == IEType::UPDATE_URR || ie.type == IEType::CREATE_URR) {
                    const auto inner = parseIEs(ie.value);
                    auto urr = URRRule::fromIEs(inner);
                    if (urr) upd_urrs.push_back(std::move(*urr));
                } else if (ie.type == IEType::UPDATE_QER || ie.type == IEType::CREATE_QER) {
                    const auto inner = parseIEs(ie.value);
                    auto qer = QERRule::fromIEs(inner);
                    if (qer) upd_qers.push_back(std::move(*qer));
                }
            }

            if (!upd_pdrs.empty() || !upd_fars.empty() ||
                !upd_urrs.empty() || !upd_qers.empty()) {
                session->updateRules(std::move(upd_pdrs), std::move(upd_fars),
                                     std::move(upd_urrs), std::move(upd_qers));
            } else {
                session->handleSessionModification();
            }
            return true;
        }

        case PFCPMessageType::SESSION_DELETION_REQUEST: {
            auto it = sessions_.find(header.seid);
            if (it == sessions_.end()) {
                std::cerr << "[PFCP] Reject SESSION_DELETION: unknown seid="
                          << header.seid << std::endl;
                return false;
            }
            // Collect final usage reports before destroying the session.
            const auto reports = it->second->collectUsageReports(false);
            if (!reports.empty()) {
                std::cout << "[PFCP] Session seid=" << header.seid
                          << " final usage reports: " << reports.size() << " URR(s)" << std::endl;
                for (const auto& rep : reports) {
                    std::cout << "[PFCP]   URR " << rep.urr_id
                              << " seqn=" << rep.ur_seqn
                              << " ul=" << rep.volume.ul_octets
                              << " dl=" << rep.volume.dl_octets << " octets" << std::endl;
                }
            }
            it->second->handleSessionDeletion();
            sessions_.erase(it);
            return true;
        }

        default:
            std::cerr << "[PFCP] Unsupported message type "
                      << static_cast<int>(header.message_type) << std::endl;
            return false;
    }
}

bool PFCPHandler::handleSessionEstablishment(const PFCPHeader& header,
                                             const std::vector<uint8_t>& payload) {
    if (!association_established_) {
        std::cerr << "[PFCP] Reject SESSION_ESTABLISHMENT seid=" << header.seid
                  << ": association not established" << std::endl;
        return false;
    }

    // Extract the subscriber identity from the AUSF_SUPI enterprise IE.
    const auto ies = parseIEs(payload);
    const auto* supi_ie = findIE(ies, IEType::AUSF_SUPI);
    if (supi_ie == nullptr) {
        std::cerr << "[PFCP] Reject SESSION_ESTABLISHMENT seid=" << header.seid
                  << ": AUSF_SUPI IE missing — cannot verify authentication" << std::endl;
        return false;
    }
    const std::string supi = supi_ie->toSupiString();

    // Gate on authentication registry: reject if not yet authenticated.
    const auto it = auth_registry_.find(supi);
    if (it == auth_registry_.end()) {
        std::cerr << "[PFCP] Reject SESSION_ESTABLISHMENT seid=" << header.seid
                  << " supi=" << supi << ": no authentication result on record" << std::endl;
        return false;
    }
    if (!it->second.isAuthenticated()) {
        std::cerr << "[PFCP] Reject SESSION_ESTABLISHMENT seid=" << header.seid
                  << " supi=" << supi
                  << ": status=" << it->second.statusString() << std::endl;
        return false;
    }

    auto session = std::make_shared<PFCPSession>(header.seid, supi, it->second);

    // Parse grouped PDR/FAR/URR/QER rules from the request payload.
    std::vector<PDRRule> pdrs;
    std::vector<FARRule> far_rules;
    std::vector<URRRule> urrs;
    std::vector<QERRule> qers;

    for (const auto& ie : ies) {
        if (ie.type == IEType::CREATE_PDR) {
            const auto inner = parseIEs(ie.value);
            auto pdr = PDRRule::fromIEs(inner);
            if (pdr) pdrs.push_back(std::move(*pdr));
        } else if (ie.type == IEType::CREATE_FAR) {
            const auto inner = parseIEs(ie.value);
            auto fr = FARRule::fromIEs(inner);
            if (fr) far_rules.push_back(std::move(*fr));
        } else if (ie.type == IEType::CREATE_URR) {
            const auto inner = parseIEs(ie.value);
            auto urr = URRRule::fromIEs(inner);
            if (urr) urrs.push_back(std::move(*urr));
        } else if (ie.type == IEType::CREATE_QER) {
            const auto inner = parseIEs(ie.value);
            auto qer = QERRule::fromIEs(inner);
            if (qer) qers.push_back(std::move(*qer));
        }
    }

    if (!pdrs.empty() || !far_rules.empty() || !urrs.empty() || !qers.empty()) {
        session->installRules(std::move(pdrs), std::move(far_rules),
                              std::move(urrs), std::move(qers));
    }

    session->handleSessionEstablishment();
    sessions_[header.seid] = session;
    return true;
}

std::vector<uint8_t> PFCPHandler::buildResponse(uint8_t msg_type,
                                                bool seid_present,
                                                uint64_t seid,
                                                uint32_t seq,
                                                const std::vector<uint8_t>& ie_payload) const {
    PFCPHeader hdr;
    hdr.version        = 1;
    hdr.message_type   = msg_type;
    hdr.seid_present   = seid_present;
    hdr.seid           = seid;
    hdr.sequence_number = seq;
    // message_length = bytes after octet 4 = (header - 4) + ie_payload
    hdr.message_length = static_cast<uint16_t>(
        (seid_present ? 12u : 4u) + ie_payload.size());
    auto pdu = encodePFCPHeader(hdr);
    pdu.insert(pdu.end(), ie_payload.begin(), ie_payload.end());
    return pdu;
}

std::string PFCPHandler::messageTypeToString(uint8_t message_type) {
    switch (static_cast<PFCPMessageType>(message_type)) {
        case PFCPMessageType::HEARTBEAT_REQUEST:              return "HEARTBEAT_REQUEST";
        case PFCPMessageType::HEARTBEAT_RESPONSE:             return "HEARTBEAT_RESPONSE";
        case PFCPMessageType::ASSOCIATION_SETUP_REQUEST:      return "ASSOCIATION_SETUP_REQUEST";
        case PFCPMessageType::ASSOCIATION_SETUP_RESPONSE:     return "ASSOCIATION_SETUP_RESPONSE";
        case PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST:  return "SESSION_ESTABLISHMENT_REQUEST";
        case PFCPMessageType::SESSION_ESTABLISHMENT_RESPONSE: return "SESSION_ESTABLISHMENT_RESPONSE";
        case PFCPMessageType::SESSION_MODIFICATION_REQUEST:   return "SESSION_MODIFICATION_REQUEST";
        case PFCPMessageType::SESSION_MODIFICATION_RESPONSE:  return "SESSION_MODIFICATION_RESPONSE";
        case PFCPMessageType::SESSION_DELETION_REQUEST:       return "SESSION_DELETION_REQUEST";
        case PFCPMessageType::SESSION_DELETION_RESPONSE:      return "SESSION_DELETION_RESPONSE";
        default:                                              return "UNKNOWN";
    }
}

} // namespace networking
} // namespace ausf
