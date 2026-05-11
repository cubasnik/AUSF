#include "pfcp_rules.h"

#include <algorithm>
#include <cstring>

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// F-TEID
// ---------------------------------------------------------------------------

std::vector<uint8_t> FTEID::encode() const {
    std::vector<uint8_t> buf;
    uint8_t flags = 0;
    if (ipv4_present) flags |= 0x01u;
    if (ipv6_present) flags |= 0x02u;
    buf.push_back(flags);

    // TEID (4 bytes big-endian)
    buf.push_back(static_cast<uint8_t>(teid >> 24));
    buf.push_back(static_cast<uint8_t>((teid >> 16) & 0xFF));
    buf.push_back(static_cast<uint8_t>((teid >>  8) & 0xFF));
    buf.push_back(static_cast<uint8_t>( teid        & 0xFF));

    if (ipv4_present) {
        buf.push_back(static_cast<uint8_t>(ipv4 >> 24));
        buf.push_back(static_cast<uint8_t>((ipv4 >> 16) & 0xFF));
        buf.push_back(static_cast<uint8_t>((ipv4 >>  8) & 0xFF));
        buf.push_back(static_cast<uint8_t>( ipv4        & 0xFF));
    }
    if (ipv6_present) {
        buf.insert(buf.end(), ipv6, ipv6 + 16);
    }
    return buf;
}

std::optional<FTEID> FTEID::decode(const std::vector<uint8_t>& value) {
    if (value.size() < 5) return std::nullopt; // flags + 4-byte TEID minimum
    FTEID f;
    const uint8_t flags = value[0];
    f.ipv4_present = (flags & 0x01u) != 0;
    f.ipv6_present = (flags & 0x02u) != 0;
    f.teid = decodeU32(value, 1);

    std::size_t off = 5;
    if (f.ipv4_present) {
        if (value.size() < off + 4) return std::nullopt;
        f.ipv4 = decodeU32(value, off);
        off += 4;
    }
    if (f.ipv6_present) {
        if (value.size() < off + 16) return std::nullopt;
        std::memcpy(f.ipv6, value.data() + off, 16);
    }
    return f;
}

FTEID FTEID::withIPv4(uint32_t teid, uint32_t ipv4_addr) {
    FTEID f;
    f.teid         = teid;
    f.ipv4_present = true;
    f.ipv4         = ipv4_addr;
    return f;
}

// ---------------------------------------------------------------------------
// PDI
// ---------------------------------------------------------------------------

std::vector<InformationElement> PDI::toIEs() const {
    std::vector<InformationElement> ies;

    // SOURCE_INTERFACE (1 byte)
    InformationElement src{};
    src.type  = IEType::SOURCE_INTERFACE;
    src.value = { static_cast<uint8_t>(source_interface) };
    ies.push_back(std::move(src));

    // F-TEID (optional)
    if (local_fteid.has_value()) {
        InformationElement fteid_ie{};
        fteid_ie.type  = IEType::F_TEID;
        fteid_ie.value = local_fteid->encode();
        ies.push_back(std::move(fteid_ie));
    }

    // NETWORK_INSTANCE (optional, UTF-8)
    if (!network_instance.empty()) {
        InformationElement ni{};
        ni.type  = IEType::NETWORK_INSTANCE;
        ni.value.assign(network_instance.begin(), network_instance.end());
        ies.push_back(std::move(ni));
    }

    return ies;
}

std::optional<PDI> PDI::fromIEs(const std::vector<InformationElement>& ies) {
    const auto* src = findIE(ies, IEType::SOURCE_INTERFACE);
    if (!src || src->value.empty()) return std::nullopt;

    PDI pdi;
    pdi.source_interface = static_cast<InterfaceType>(src->value[0]);

    if (const auto* ft = findIE(ies, IEType::F_TEID)) {
        pdi.local_fteid = FTEID::decode(ft->value);
    }
    if (const auto* ni = findIE(ies, IEType::NETWORK_INSTANCE)) {
        pdi.network_instance.assign(ni->value.begin(), ni->value.end());
    }
    return pdi;
}

// ---------------------------------------------------------------------------
// PDRRule
// ---------------------------------------------------------------------------

std::vector<InformationElement> PDRRule::toIEs() const {
    std::vector<InformationElement> ies;

    InformationElement id_ie{};
    id_ie.type  = IEType::PDR_ID;
    id_ie.value = encodeU16(pdr_id);
    ies.push_back(std::move(id_ie));

    InformationElement prec{};
    prec.type  = IEType::PRECEDENCE;
    prec.value = encodeU32(precedence);
    ies.push_back(std::move(prec));

    // PDI is encoded as a grouped IE (CREATE_PDR carries it as nested IEs;
    // we encode PDI inner IEs and wrap them as a PDI grouped IE).
    const auto pdi_ies   = pdi.toIEs();
    const auto pdi_bytes = encodeIEs(pdi_ies);
    InformationElement pdi_ie{};
    pdi_ie.type  = IEType::PDI;
    pdi_ie.value = pdi_bytes;
    ies.push_back(std::move(pdi_ie));

    InformationElement far_ref{};
    far_ref.type  = IEType::FAR_ID;
    far_ref.value = encodeU32(far_id);
    ies.push_back(std::move(far_ref));

    // QER_ID (optional — present only when a QER is associated)
    if (qer_id != 0) {
        InformationElement qer_ref{};
        qer_ref.type  = IEType::QER_ID;
        qer_ref.value = encodeU32(qer_id);
        ies.push_back(std::move(qer_ref));
    }

    return ies;
}

std::optional<PDRRule> PDRRule::fromIEs(const std::vector<InformationElement>& ies) {
    const auto* id_ie = findIE(ies, IEType::PDR_ID);
    const auto* prec  = findIE(ies, IEType::PRECEDENCE);
    const auto* pdi_ie = findIE(ies, IEType::PDI);
    const auto* far_ref = findIE(ies, IEType::FAR_ID);
    if (!id_ie || !prec || !pdi_ie || !far_ref) return std::nullopt;

    PDRRule pdr;
    pdr.pdr_id     = decodeU16(id_ie->value);
    pdr.precedence = decodeU32(prec->value);
    pdr.far_id     = decodeU32(far_ref->value);

    const auto pdi_ies = parseIEs(pdi_ie->value);
    auto parsed_pdi = PDI::fromIEs(pdi_ies);
    if (!parsed_pdi) return std::nullopt;
    pdr.pdi = std::move(*parsed_pdi);

    // QER_ID (optional)
    if (const auto* qer_ref = findIE(ies, IEType::QER_ID)) {
        pdr.qer_id = decodeU32(qer_ref->value);
    }

    return pdr;
}

// ---------------------------------------------------------------------------
// ApplyAction
// ---------------------------------------------------------------------------

uint8_t ApplyAction::encode() const noexcept {
    uint8_t b = 0;
    if (drop) b |= 0x01u;
    if (forw) b |= 0x02u;
    if (buff) b |= 0x04u;
    if (nocp) b |= 0x08u;
    if (dupl) b |= 0x10u;
    return b;
}

ApplyAction ApplyAction::decode(uint8_t byte) noexcept {
    ApplyAction a;
    a.drop = (byte & 0x01u) != 0;
    a.forw = (byte & 0x02u) != 0;
    a.buff = (byte & 0x04u) != 0;
    a.nocp = (byte & 0x08u) != 0;
    a.dupl = (byte & 0x10u) != 0;
    return a;
}

// ---------------------------------------------------------------------------
// ForwardingParameters
// ---------------------------------------------------------------------------

std::vector<InformationElement> ForwardingParameters::toIEs() const {
    std::vector<InformationElement> ies;

    InformationElement dst{};
    dst.type  = IEType::DESTINATION_INTERFACE;
    dst.value = { static_cast<uint8_t>(destination_interface) };
    ies.push_back(std::move(dst));

    if (!network_instance.empty()) {
        InformationElement ni{};
        ni.type  = IEType::NETWORK_INSTANCE;
        ni.value.assign(network_instance.begin(), network_instance.end());
        ies.push_back(std::move(ni));
    }

    if (outer_header_creation.has_value()) {
        InformationElement ohc{};
        ohc.type  = IEType::F_TEID; // reuse F-TEID for outer header creation target
        ohc.value = outer_header_creation->encode();
        ies.push_back(std::move(ohc));
    }

    return ies;
}

std::optional<ForwardingParameters> ForwardingParameters::fromIEs(
        const std::vector<InformationElement>& ies) {
    const auto* dst = findIE(ies, IEType::DESTINATION_INTERFACE);
    if (!dst || dst->value.empty()) return std::nullopt;

    ForwardingParameters fp;
    fp.destination_interface = static_cast<InterfaceType>(dst->value[0]);

    if (const auto* ni = findIE(ies, IEType::NETWORK_INSTANCE)) {
        fp.network_instance.assign(ni->value.begin(), ni->value.end());
    }
    if (const auto* ft = findIE(ies, IEType::F_TEID)) {
        fp.outer_header_creation = FTEID::decode(ft->value);
    }
    return fp;
}

// ---------------------------------------------------------------------------
// FARRule
// ---------------------------------------------------------------------------

std::vector<InformationElement> FARRule::toIEs() const {
    std::vector<InformationElement> ies;

    InformationElement id_ie{};
    id_ie.type  = IEType::FAR_ID;
    id_ie.value = encodeU32(far_id);
    ies.push_back(std::move(id_ie));

    InformationElement aa{};
    aa.type  = IEType::APPLY_ACTION;
    aa.value = { apply_action.encode() };
    ies.push_back(std::move(aa));

    if (forwarding_parameters.has_value()) {
        const auto fp_ies   = forwarding_parameters->toIEs();
        const auto fp_bytes = encodeIEs(fp_ies);
        InformationElement fp_ie{};
        fp_ie.type  = IEType::FORWARDING_PARAMETERS;
        fp_ie.value = fp_bytes;
        ies.push_back(std::move(fp_ie));
    }

    return ies;
}

std::optional<FARRule> FARRule::fromIEs(const std::vector<InformationElement>& ies) {
    const auto* id_ie = findIE(ies, IEType::FAR_ID);
    const auto* aa_ie = findIE(ies, IEType::APPLY_ACTION);
    if (!id_ie || !aa_ie || aa_ie->value.empty()) return std::nullopt;

    FARRule far;
    far.far_id       = decodeU32(id_ie->value);
    far.apply_action = ApplyAction::decode(aa_ie->value[0]);

    if (const auto* fp_ie = findIE(ies, IEType::FORWARDING_PARAMETERS)) {
        const auto fp_ies = parseIEs(fp_ie->value);
        far.forwarding_parameters = ForwardingParameters::fromIEs(fp_ies);
    }
    return far;
}

// ---------------------------------------------------------------------------
// MeasurementMethod
// ---------------------------------------------------------------------------

uint8_t MeasurementMethod::encode() const noexcept {
    uint8_t b = 0;
    if (durat)  b |= 0x01u;
    if (volume) b |= 0x02u;
    if (event)  b |= 0x04u;
    return b;
}

MeasurementMethod MeasurementMethod::decode(uint8_t byte) noexcept {
    MeasurementMethod m;
    m.durat  = (byte & 0x01u) != 0;
    m.volume = (byte & 0x02u) != 0;
    m.event  = (byte & 0x04u) != 0;
    return m;
}

// ---------------------------------------------------------------------------
// ReportingTriggers
// ---------------------------------------------------------------------------

uint8_t ReportingTriggers::encode() const noexcept {
    uint8_t b = 0;
    if (perio) b |= 0x01u;
    if (volth) b |= 0x02u;
    if (timth) b |= 0x04u;
    if (quhti) b |= 0x08u;
    if (start) b |= 0x10u;
    if (stopt) b |= 0x20u;
    if (droth) b |= 0x40u;
    if (liusa) b |= 0x80u;
    return b;
}

ReportingTriggers ReportingTriggers::decode(uint8_t byte) noexcept {
    ReportingTriggers r;
    r.perio = (byte & 0x01u) != 0;
    r.volth = (byte & 0x02u) != 0;
    r.timth = (byte & 0x04u) != 0;
    r.quhti = (byte & 0x08u) != 0;
    r.start = (byte & 0x10u) != 0;
    r.stopt = (byte & 0x20u) != 0;
    r.droth = (byte & 0x40u) != 0;
    r.liusa = (byte & 0x80u) != 0;
    return r;
}

// ---------------------------------------------------------------------------
// URRRule
// ---------------------------------------------------------------------------

std::vector<InformationElement> URRRule::toIEs() const {
    std::vector<InformationElement> ies;

    InformationElement id_ie{};
    id_ie.type  = IEType::URR_ID;
    id_ie.value = encodeU32(urr_id);
    ies.push_back(std::move(id_ie));

    InformationElement mm{};
    mm.type  = IEType::MEASUREMENT_METHOD;
    mm.value = { measurement_method.encode() };
    ies.push_back(std::move(mm));

    InformationElement rt{};
    rt.type  = IEType::REPORTING_TRIGGERS;
    rt.value = { reporting_triggers.encode() };
    ies.push_back(std::move(rt));

    if (measurement_period != 0) {
        InformationElement mp{};
        mp.type  = IEType::MEASUREMENT_PERIOD;
        mp.value = encodeU32(measurement_period);
        ies.push_back(std::move(mp));
    }

    if (volume_threshold.has_value()) {
        InformationElement vt{};
        vt.type  = IEType::VOLUME_THRESHOLD;
        vt.value = volume_threshold->encode();
        ies.push_back(std::move(vt));
    }

    return ies;
}

std::optional<URRRule> URRRule::fromIEs(const std::vector<InformationElement>& ies) {
    const auto* id_ie = findIE(ies, IEType::URR_ID);
    const auto* mm_ie = findIE(ies, IEType::MEASUREMENT_METHOD);
    const auto* rt_ie = findIE(ies, IEType::REPORTING_TRIGGERS);
    if (!id_ie || !mm_ie || !rt_ie) return std::nullopt;
    if (mm_ie->value.empty() || rt_ie->value.empty()) return std::nullopt;

    URRRule urr;
    urr.urr_id             = decodeU32(id_ie->value);
    urr.measurement_method = MeasurementMethod::decode(mm_ie->value[0]);
    urr.reporting_triggers = ReportingTriggers::decode(rt_ie->value[0]);

    if (const auto* mp = findIE(ies, IEType::MEASUREMENT_PERIOD)) {
        urr.measurement_period = decodeU32(mp->value);
    }
    if (const auto* vt = findIE(ies, IEType::VOLUME_THRESHOLD)) {
        urr.volume_threshold = VolumeMeasurement::decode(vt->value);
    }
    return urr;
}

// ---------------------------------------------------------------------------
// VolumeMeasurement  (IE type 66) — TS 29.244 §8.2.44
// ---------------------------------------------------------------------------

static void encodeU64(uint64_t v, std::vector<uint8_t>& out) {
    for (int shift = 56; shift >= 0; shift -= 8)
        out.push_back(static_cast<uint8_t>((v >> shift) & 0xFFu));
}

static uint64_t readU64(const uint8_t* p) {
    uint64_t v = 0;
    for (int i = 0; i < 8; ++i)
        v = (v << 8u) | static_cast<uint64_t>(p[i]);
    return v;
}

std::vector<uint8_t> VolumeMeasurement::encode() const {
    std::vector<uint8_t> buf;
    uint8_t flags = 0;
    if (total_present) flags |= 0x01u;
    if (ul_present)    flags |= 0x02u;
    if (dl_present)    flags |= 0x04u;
    buf.push_back(flags);
    if (total_present) encodeU64(total_octets, buf);
    if (ul_present)    encodeU64(ul_octets,    buf);
    if (dl_present)    encodeU64(dl_octets,    buf);
    return buf;
}

std::optional<VolumeMeasurement> VolumeMeasurement::decode(
        const std::vector<uint8_t>& value) {
    if (value.empty()) return std::nullopt;
    VolumeMeasurement vm;
    const uint8_t flags = value[0];
    vm.total_present = (flags & 0x01u) != 0;
    vm.ul_present    = (flags & 0x02u) != 0;
    vm.dl_present    = (flags & 0x04u) != 0;

    const std::size_t needed = 1u
        + (vm.total_present ? 8u : 0u)
        + (vm.ul_present    ? 8u : 0u)
        + (vm.dl_present    ? 8u : 0u);
    if (value.size() < needed) return std::nullopt;

    std::size_t pos = 1;
    if (vm.total_present) { vm.total_octets = readU64(value.data() + pos); pos += 8; }
    if (vm.ul_present)    { vm.ul_octets    = readU64(value.data() + pos); pos += 8; }
    if (vm.dl_present)    { vm.dl_octets    = readU64(value.data() + pos); }
    return vm;
}

VolumeMeasurement VolumeMeasurement::withULDL(uint64_t ul, uint64_t dl) {
    VolumeMeasurement vm;
    vm.total_present = true;
    vm.ul_present    = true;
    vm.dl_present    = true;
    vm.total_octets  = ul + dl;
    vm.ul_octets     = ul;
    vm.dl_octets     = dl;
    return vm;
}

// ---------------------------------------------------------------------------
// UsageReport
// ---------------------------------------------------------------------------

std::vector<InformationElement> UsageReport::toIEs() const {
    std::vector<InformationElement> ies;

    // URR_ID (4 bytes big-endian)
    InformationElement urr_id_ie{};
    urr_id_ie.type  = IEType::URR_ID;
    urr_id_ie.value = encodeU32(urr_id);
    ies.push_back(std::move(urr_id_ie));

    // UR-SEQN (4 bytes big-endian)
    InformationElement seqn_ie{};
    seqn_ie.type  = IEType::UR_SEQN;
    seqn_ie.value = encodeU32(ur_seqn);
    ies.push_back(std::move(seqn_ie));

    // Usage Report Trigger (same encoding as REPORTING_TRIGGERS)
    InformationElement trig_ie{};
    trig_ie.type  = IEType::REPORTING_TRIGGERS;
    trig_ie.value = { trigger.encode() };
    ies.push_back(std::move(trig_ie));

    // Volume Measurement
    InformationElement vol_ie{};
    vol_ie.type  = IEType::VOLUME_MEASUREMENT;
    vol_ie.value = volume.encode();
    ies.push_back(std::move(vol_ie));

    return ies;
}

// ---------------------------------------------------------------------------
// QERRule  (IE type 7/14) — TS 29.244 §7.5.2.7
// ---------------------------------------------------------------------------

std::vector<InformationElement> QERRule::toIEs() const {
    std::vector<InformationElement> ies;

    // QER_ID (4 bytes big-endian)
    InformationElement id_ie{};
    id_ie.type  = IEType::QER_ID;
    id_ie.value = encodeU32(qer_id);
    ies.push_back(std::move(id_ie));

    // GATE_STATUS (1 byte): bits[1:0]=UL gate, bits[3:2]=DL gate
    //   OPEN=0, CLOSED=1
    InformationElement gate_ie{};
    gate_ie.type  = IEType::GATE_STATUS;
    gate_ie.value = {
        static_cast<uint8_t>(
            (static_cast<uint8_t>(ul_gate) & 0x03u) |
            ((static_cast<uint8_t>(dl_gate) & 0x03u) << 2))
    };
    ies.push_back(std::move(gate_ie));

    // MBR (10 bytes: UL 5 bytes + DL 5 bytes, big-endian, Kbps) — if either is set
    if (ul_mbr != 0 || dl_mbr != 0) {
        InformationElement mbr_ie{};
        mbr_ie.type = IEType::MBR;
        auto ul_bytes = encodeU40(ul_mbr);
        auto dl_bytes = encodeU40(dl_mbr);
        mbr_ie.value.insert(mbr_ie.value.end(), ul_bytes.begin(), ul_bytes.end());
        mbr_ie.value.insert(mbr_ie.value.end(), dl_bytes.begin(), dl_bytes.end());
        ies.push_back(std::move(mbr_ie));
    }

    // GBR (10 bytes: UL 5 bytes + DL 5 bytes) — if either is set
    if (ul_gbr != 0 || dl_gbr != 0) {
        InformationElement gbr_ie{};
        gbr_ie.type = IEType::GBR;
        auto ul_bytes = encodeU40(ul_gbr);
        auto dl_bytes = encodeU40(dl_gbr);
        gbr_ie.value.insert(gbr_ie.value.end(), ul_bytes.begin(), ul_bytes.end());
        gbr_ie.value.insert(gbr_ie.value.end(), dl_bytes.begin(), dl_bytes.end());
        ies.push_back(std::move(gbr_ie));
    }

    return ies;
}

std::optional<QERRule> QERRule::fromIEs(const std::vector<InformationElement>& ies) {
    const auto* id_ie   = findIE(ies, IEType::QER_ID);
    const auto* gate_ie = findIE(ies, IEType::GATE_STATUS);
    if (!id_ie || !gate_ie) return std::nullopt;

    QERRule qer;
    qer.qer_id = decodeU32(id_ie->value);

    const uint8_t gate_byte = gate_ie->value.empty() ? 0u : gate_ie->value[0];
    qer.ul_gate = ((gate_byte & 0x03u) != 0) ? GateStatus::CLOSED : GateStatus::OPEN;
    qer.dl_gate = (((gate_byte >> 2) & 0x03u) != 0) ? GateStatus::CLOSED : GateStatus::OPEN;

    if (const auto* mbr_ie = findIE(ies, IEType::MBR)) {
        if (mbr_ie->value.size() >= 10) {
            qer.ul_mbr = decodeU40(mbr_ie->value, 0);
            qer.dl_mbr = decodeU40(mbr_ie->value, 5);
        }
    }

    if (const auto* gbr_ie = findIE(ies, IEType::GBR)) {
        if (gbr_ie->value.size() >= 10) {
            qer.ul_gbr = decodeU40(gbr_ie->value, 0);
            qer.dl_gbr = decodeU40(gbr_ie->value, 5);
        }
    }

    return qer;
}

} // namespace networking
} // namespace ausf
