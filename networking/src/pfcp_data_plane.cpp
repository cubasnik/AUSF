#include "pfcp_data_plane.h"

#include <iostream>

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// Session registration / deregistration
// ---------------------------------------------------------------------------

void DataPlane::addSession(std::shared_ptr<PFCPSession> session) {
    std::lock_guard<std::mutex> lock(mutex_);

    const uint64_t seid = session->getSEID();
    seid_index_[seid] = session;

    // Build the TEID index from every PDR that has a local F-TEID.
    for (const auto& pdr : session->getPDRs()) {
        if (pdr.pdi.local_fteid.has_value()) {
            teid_index_[pdr.pdi.local_fteid->teid] = session;
        }
    }

    std::cout << "[DataPlane] addSession seid=" << seid
              << " teid_entries=" << session->getPDRs().size() << std::endl;
}

void DataPlane::removeSession(uint64_t seid) {
    std::lock_guard<std::mutex> lock(mutex_);

    auto sit = seid_index_.find(seid);
    if (sit == seid_index_.end()) return;

    for (const auto& pdr : sit->second->getPDRs()) {
        if (pdr.pdi.local_fteid.has_value()) {
            teid_index_.erase(pdr.pdi.local_fteid->teid);
        }
    }
    buffer_.erase(seid);
    seid_index_.erase(sit);

    std::cout << "[DataPlane] removeSession seid=" << seid << std::endl;
}

std::size_t DataPlane::sessionCount() const {
    std::lock_guard<std::mutex> lock(mutex_);
    return seid_index_.size();
}

// ---------------------------------------------------------------------------
// Uplink processing
// ---------------------------------------------------------------------------

ForwardResult DataPlane::processUplink(const GTPUFrame& frame) {
    ForwardResult result;
    const uint32_t incoming_teid = frame.header.teid;

    std::lock_guard<std::mutex> lock(mutex_);

    auto tit = teid_index_.find(incoming_teid);
    if (tit == teid_index_.end()) {
        std::cerr << "[DataPlane] No PDR match for teid=0x"
                  << std::hex << incoming_teid << std::dec << std::endl;
        return result; // no match — caller may log/drop
    }

    auto& session = tit->second;
    result.session_seid = session->getSEID();

    // --- Step 1: find best-precedence PDR for this TEID ---
    const PDRRule* pdr = matchPDR(*session, incoming_teid);
    if (!pdr) return result;
    result.pdr_id = pdr->pdr_id;

    // --- Step 2: resolve FAR ---
    const FARRule* far_rule = findFAR(*session, pdr->far_id);
    if (!far_rule) {
        std::cerr << "[DataPlane] FAR id=" << pdr->far_id
                  << " not found in session seid=" << session->getSEID() << std::endl;
        return result;
    }
    result.far_id = far_rule->far_id;

    const uint64_t pkt_bytes  = static_cast<uint64_t>(frame.payload.size());

    // --- Step 2.5: QER gate check (uplink) ---
    if (pdr->qer_id != 0) {
        const QERRule* qer = session->findQER(pdr->qer_id);
        if (qer && !qer->isUplinkOpen()) {
            result.gated   = true;
            result.dropped = true;
            session->accountUplinkOctets(pkt_bytes);
            std::cout << "[DataPlane] seid=" << session->getSEID()
                      << " GATED (UL gate closed) qer=" << pdr->qer_id
                      << " teid=0x" << std::hex << incoming_teid << std::dec << std::endl;
            return result;
        }
    }

    // --- Step 3: apply FAR action ---
    if (far_rule->apply_action.drop) {
        result.dropped = true;
        session->accountUplinkOctets(pkt_bytes);
        std::cout << "[DataPlane] seid=" << session->getSEID()
                  << " DROP " << pkt_bytes << " B teid=0x"
                  << std::hex << incoming_teid << std::dec << std::endl;
        return result;
    }

    if (far_rule->apply_action.buff) {
        result.buffered = true;
        session->accountUplinkOctets(pkt_bytes);
        buffer_[session->getSEID()].push_back(frame.payload);
        std::cout << "[DataPlane] seid=" << session->getSEID()
                  << " BUFF " << pkt_bytes << " B (buffer size="
                  << buffer_[session->getSEID()].size() << ")" << std::endl;
        return result;
    }

    if (far_rule->apply_action.forw) {
        session->accountUplinkOctets(pkt_bytes);

        // Determine outgoing TEID.
        // If ForwardingParameters carries an outer_header_creation F-TEID, use its TEID;
        // otherwise re-use the incoming TEID (transparent tunnel).
        uint32_t out_teid = incoming_teid;
        if (far_rule->forwarding_parameters.has_value() &&
            far_rule->forwarding_parameters->outer_header_creation.has_value()) {
            out_teid = far_rule->forwarding_parameters->outer_header_creation->teid;
        }

        GTPUHeader out_hdr{};
        out_hdr.version      = 1;
        out_hdr.pt           = true;
        out_hdr.message_type = GTPUMessageType::G_PDU;
        out_hdr.teid         = out_teid;

        result.forwarded = true;
        result.frame     = encodeGTPU(out_hdr, frame.payload);

        std::cout << "[DataPlane] seid=" << session->getSEID()
                  << " FORW " << pkt_bytes << " B"
                  << " teid_in=0x"  << std::hex << incoming_teid
                  << " teid_out=0x" << out_teid  << std::dec << std::endl;
        return result;
    }

    // Action not handled (nocp, dupl etc.) — log and return no-op
    std::cout << "[DataPlane] seid=" << session->getSEID()
              << " action=0x" << std::hex
              << static_cast<unsigned>(far_rule->apply_action.encode())
              << std::dec << " not handled" << std::endl;
    return result;
}

// ---------------------------------------------------------------------------
// Private helpers (caller must hold mutex_)
// ---------------------------------------------------------------------------

const PDRRule* DataPlane::matchPDR(const PFCPSession& session, uint32_t teid) const {
    const PDRRule* best = nullptr;
    for (const auto& pdr : session.getPDRs()) {
        if (pdr.pdi.local_fteid.has_value() &&
            pdr.pdi.local_fteid->teid == teid) {
            if (!best || pdr.precedence < best->precedence) {
                best = &pdr;
            }
        }
    }
    return best;
}

const FARRule* DataPlane::findFAR(const PFCPSession& session, uint32_t far_id) const {
    for (const auto& f : session.getFARs()) {
        if (f.far_id == far_id) return &f;
    }
    return nullptr;
}

// ---------------------------------------------------------------------------
// Buffer management
// ---------------------------------------------------------------------------

std::vector<std::vector<uint8_t>> DataPlane::drainBuffer(uint64_t seid) {
    std::lock_guard<std::mutex> lock(mutex_);
    auto it = buffer_.find(seid);
    if (it == buffer_.end()) return {};
    std::vector<std::vector<uint8_t>> result = std::move(it->second);
    buffer_.erase(it);
    return result;
}

} // namespace networking
} // namespace ausf
