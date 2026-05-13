#pragma once

// PFCP session rules — TS 29.244 §7.5
//
// This header defines C++ representations for the three core rule types that
// the UPF must install on session establishment:
//
//   PDRRule  – Packet Detection Rule  (§7.5.2.2)
//   FARRule  – Forwarding Action Rule  (§7.5.2.3)
//   URRRule  – Usage Reporting Rule    (§7.5.2.4)
//
// Each rule struct maps closely to the grouped IEs defined in the spec.
// encode*() / decode*() functions convert between the struct representation
// and a flat IE byte buffer (inner IEs, without the outer grouped-IE wrapper)
// so that callers can embed them inside CREATE_PDR / CREATE_FAR / CREATE_URR
// grouped IEs.

#include "pfcp_ie.h"

#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// Source / Destination interface values (IE type 20 / 42)
// TS 29.244 Table 7.5.2.2-1
// ---------------------------------------------------------------------------
enum class InterfaceType : uint8_t {
    ACCESS        = 0,  // Uu interface (toward UE)
    CORE          = 1,  // N6 interface (toward DN)
    SGI_LAN_N6_LAN = 2,
    CP_FUNCTION   = 3,
    LI_FUNCTION   = 4,
};

// ---------------------------------------------------------------------------
// F-TEID  (IE type 21) — TS 29.244 §8.2.3
//
// Wire encoding (4 bytes fixed + optional fields):
//   Octet 1: spare(4) | CH | CHID | V6 | V4
//   Octet 2-5: TEID (big-endian), always present
//   Octet 6-9 (V4=1): IPv4 address
//   Octet 10-25 (V6=1): IPv6 address (16 bytes)
//   Octet (optional): CHOOSE ID (1 byte, when CHID=1)
// ---------------------------------------------------------------------------
struct FTEID {
    uint32_t    teid        = 0;
    bool        ipv4_present = false;
    bool        ipv6_present = false;
    uint32_t    ipv4        = 0;    // big-endian host-order (network byte order stored)
    uint8_t     ipv6[16]    = {};

    // Encode to IE value bytes (without the 4-byte TLV outer header).
    std::vector<uint8_t> encode() const;
    static std::optional<FTEID> decode(const std::vector<uint8_t>& value);

    // Build a minimal IPv4-only F-TEID.
    static FTEID withIPv4(uint32_t teid, uint32_t ipv4_addr);
};

// ---------------------------------------------------------------------------
// PDI — Packet Detection Information  (grouped IE, §7.5.2.2)
// ---------------------------------------------------------------------------
struct PDI {
    InterfaceType        source_interface = InterfaceType::ACCESS;
    std::optional<FTEID> local_fteid;          // optional for UL PDRRule
    std::string          network_instance;     // DNN/APN, may be empty

    // Encode inner IEs of PDI (used as value of CREATE_PDR grouped IE).
    std::vector<InformationElement> toIEs() const;
    static std::optional<PDI> fromIEs(const std::vector<InformationElement>& ies);
};

// ---------------------------------------------------------------------------
// PDRRule — Packet Detection Rule  (§7.5.2.2)
// ---------------------------------------------------------------------------
struct PDRRule {
    uint16_t pdr_id     = 0;
    uint32_t precedence = 0;
    PDI      pdi;
    uint32_t far_id     = 0;    // FARRule to execute when this PDRRule matches
    uint32_t qer_id     = 0;    // QERRule to apply (0 = none)

    // Encode as inner IEs of CREATE_PDR grouped IE.
    std::vector<InformationElement> toIEs() const;
    static std::optional<PDRRule> fromIEs(const std::vector<InformationElement>& ies);
};

// ---------------------------------------------------------------------------
// Apply Action flags  (IE type 44)  — TS 29.244 Table 7.5.2.3-3
// ---------------------------------------------------------------------------
struct ApplyAction {
    bool drop  = false;   // bit 0
    bool forw  = false;   // bit 1 — forward
    bool buff  = false;   // bit 2 — buffer
    bool nocp  = false;   // bit 3 — notify CP
    bool dupl  = false;   // bit 4 — duplicate

    uint8_t encode() const noexcept;
    static ApplyAction decode(uint8_t byte) noexcept;
};

// ---------------------------------------------------------------------------
// Forwarding Parameters  (grouped IE, §7.5.2.3-1)
// ---------------------------------------------------------------------------
struct ForwardingParameters {
    InterfaceType        destination_interface = InterfaceType::CORE;
    std::string          network_instance;     // optional, may be empty
    std::optional<FTEID> outer_header_creation; // for GTP-U encap (UL)

    std::vector<InformationElement> toIEs() const;
    static std::optional<ForwardingParameters> fromIEs(const std::vector<InformationElement>& ies);
};

// ---------------------------------------------------------------------------
// FARRule — Forwarding Action Rule  (§7.5.2.3)
// ---------------------------------------------------------------------------
struct FARRule {
    uint32_t              far_id = 0;
    ApplyAction           apply_action;
    std::optional<ForwardingParameters> forwarding_parameters;

    std::vector<InformationElement> toIEs() const;
    static std::optional<FARRule> fromIEs(const std::vector<InformationElement>& ies);
};

// ---------------------------------------------------------------------------
// Measurement Method flags  (IE type 62)  — TS 29.244 §8.2.40
// ---------------------------------------------------------------------------
struct MeasurementMethod {
    bool durat  = false;  // duration
    bool volume = false;  // volume
    bool event  = false;  // event

    uint8_t encode() const noexcept;
    static MeasurementMethod decode(uint8_t byte) noexcept;
};

// ---------------------------------------------------------------------------
// Reporting Triggers  (IE type 63)  — TS 29.244 §8.2.41
// ---------------------------------------------------------------------------
struct ReportingTriggers {
    bool perio  = false;  // periodic
    bool volth  = false;  // volume threshold
    bool timth  = false;  // time threshold
    bool quhti  = false;  // quota holding time
    bool start  = false;  // start of traffic
    bool stopt  = false;  // stop of traffic
    bool droth  = false;  // dropped DL traffic threshold
    bool liusa  = false;  // linked usage reporting

    uint8_t encode() const noexcept;
    static ReportingTriggers decode(uint8_t byte) noexcept;
};

// ---------------------------------------------------------------------------
// VolumeMeasurement  (IE type 66) — TS 29.244 §8.2.44
//
// Wire encoding:
//   Octet 1: spare(5) | DLVOL | ULVOL | TOVOL
//   Octets 2-9   (TOVOL=1): total octets (big-endian uint64)
//   Octets +8    (ULVOL=1): UL octets (big-endian uint64)
//   Octets +8    (DLVOL=1): DL octets (big-endian uint64)
// ---------------------------------------------------------------------------
struct VolumeMeasurement {
    bool     total_present = false;
    bool     ul_present    = false;
    bool     dl_present    = false;
    uint64_t total_octets  = 0;
    uint64_t ul_octets     = 0;
    uint64_t dl_octets     = 0;

    std::vector<uint8_t> encode() const;
    static std::optional<VolumeMeasurement> decode(const std::vector<uint8_t>& value);

    // Convenience: build a measurement with UL + DL (and total = UL + DL).
    static VolumeMeasurement withULDL(uint64_t ul, uint64_t dl);
};

// ---------------------------------------------------------------------------
// URRRule — Usage Reporting Rule  (§7.5.2.4)
// ---------------------------------------------------------------------------
struct URRRule {
    uint32_t           urr_id              = 0;
    MeasurementMethod  measurement_method;
    ReportingTriggers  reporting_triggers;
    uint32_t           measurement_period  = 0; // seconds; 0 = not set

    // Volume threshold that triggers a VOLTH usage report (IE type 31).
    // Uses VolumeMeasurement wire format (same flags+counters layout).
    // std::nullopt = no threshold configured.
    std::optional<VolumeMeasurement> volume_threshold;

    std::vector<InformationElement> toIEs() const;
    static std::optional<URRRule> fromIEs(const std::vector<InformationElement>& ies);
};

// ---------------------------------------------------------------------------
// Gate Status values  (IE type 25)  — TS 29.244 §8.2.26
//
// Wire: 1 byte.  Bits [1:0] = UL gate (00=OPEN, 01=CLOSED).
//                Bits [3:2] = DL gate (00=OPEN, 01=CLOSED).
// ---------------------------------------------------------------------------
enum class GateStatus : uint8_t {
    OPEN   = 0,
    CLOSED = 1,
};

// ---------------------------------------------------------------------------
// QERRule — Quality Enforcement Rule  (§7.5.2.7, grouped IE type 7)
//
// Controls per-flow gating and bitrate enforcement.  A PDRRule may reference
// one QER via its qer_id; the data plane drops the packet if the relevant
// gate is CLOSED.
// ---------------------------------------------------------------------------
struct QERRule {
    uint32_t   qer_id  = 0;
    GateStatus ul_gate = GateStatus::OPEN;   // uplink gate
    GateStatus dl_gate = GateStatus::OPEN;   // downlink gate
    uint64_t   ul_mbr  = 0;   // Maximum Bitrate uplink   (Kbps); 0 = not set
    uint64_t   dl_mbr  = 0;   // Maximum Bitrate downlink (Kbps); 0 = not set
    uint64_t   ul_gbr  = 0;   // Guaranteed Bitrate uplink   (Kbps); 0 = not set
    uint64_t   dl_gbr  = 0;   // Guaranteed Bitrate downlink (Kbps); 0 = not set

    bool isUplinkOpen()   const noexcept { return ul_gate == GateStatus::OPEN; }
    bool isDownlinkOpen() const noexcept { return dl_gate == GateStatus::OPEN; }

    // Encode as inner IEs of CREATE_QER / UPDATE_QER grouped IE.
    std::vector<InformationElement> toIEs() const;
    static std::optional<QERRule> fromIEs(const std::vector<InformationElement>& ies);
};

// ---------------------------------------------------------------------------
// Helpers: encode uint32/uint16 big-endian into IE value bytes
// ---------------------------------------------------------------------------
inline std::vector<uint8_t> encodeU32(uint32_t v) {
    return { static_cast<uint8_t>(v >> 24), static_cast<uint8_t>((v >> 16) & 0xFF),
             static_cast<uint8_t>((v >>  8) & 0xFF), static_cast<uint8_t>(v & 0xFF) };
}
inline std::vector<uint8_t> encodeU16(uint16_t v) {
    return { static_cast<uint8_t>(v >> 8), static_cast<uint8_t>(v & 0xFF) };
}
inline uint32_t decodeU32(const std::vector<uint8_t>& v, std::size_t off = 0) {
    if (v.size() < off + 4) return 0;
    return (static_cast<uint32_t>(v[off]) << 24) | (static_cast<uint32_t>(v[off+1]) << 16)
         | (static_cast<uint32_t>(v[off+2]) << 8)  |  static_cast<uint32_t>(v[off+3]);
}
inline uint16_t decodeU16(const std::vector<uint8_t>& v, std::size_t off = 0) {
    if (v.size() < off + 2) return 0;
    return static_cast<uint16_t>((static_cast<uint16_t>(v[off]) << 8) | v[off+1]);
}
// 5-byte (40-bit) encode/decode used for MBR and GBR bitrate fields (TS 29.244 §8.2.6).
inline std::vector<uint8_t> encodeU40(uint64_t v) {
    return { static_cast<uint8_t>((v >> 32) & 0xFF),
             static_cast<uint8_t>((v >> 24) & 0xFF),
             static_cast<uint8_t>((v >> 16) & 0xFF),
             static_cast<uint8_t>((v >>  8) & 0xFF),
             static_cast<uint8_t>( v        & 0xFF) };
}
inline uint64_t decodeU40(const std::vector<uint8_t>& v, std::size_t off = 0) {
    if (v.size() < off + 5) return 0;
    return (static_cast<uint64_t>(v[off])   << 32)
         | (static_cast<uint64_t>(v[off+1]) << 24)
         | (static_cast<uint64_t>(v[off+2]) << 16)
         | (static_cast<uint64_t>(v[off+3]) <<  8)
         |  static_cast<uint64_t>(v[off+4]);
}

// ---------------------------------------------------------------------------
// UsageReport — produced by a URR when a reporting trigger fires.
// TS 29.244 §7.5.8 (within Session Modification / Deletion Response)
//
// The caller embeds the inner IEs returned by toIEs() inside a grouped IE of
// the appropriate type (USAGE_REPORT_IN_SESSION_MODIFICATION_RESPONSE etc.).
// ---------------------------------------------------------------------------
struct UsageReport {
    uint32_t          urr_id  = 0;
    uint32_t          ur_seqn = 0;  // monotonically increasing per URR
    ReportingTriggers trigger;       // which trigger fired (e.g. perio=true)
    VolumeMeasurement volume;

    // Encode as inner IEs: URR_ID + UR_SEQN + REPORTING_TRIGGERS + VOLUME_MEASUREMENT.
    std::vector<InformationElement> toIEs() const;
};

} // namespace networking
} // namespace ausf
