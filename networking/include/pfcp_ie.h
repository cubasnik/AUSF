#pragma once

#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// PFCP PDU header — TS 29.244 §7.2.3
//
// Wire layout (mandatory part, 8 bytes):
//   Octet 1:  [Ver(3) | spare(2) | FO | MP | S]
//             Ver=1, FO=0 (no follow-on), MP=0, S=1 when SEID present
//   Octet 2:  Message Type
//   Octet 3-4: Message Length (length of PDU after octet 4, big-endian)
//   Octet 5-12 (when S=1): SEID (8 bytes, big-endian)
//   Octet 5 or 13: Sequence Number (3 bytes, big-endian) + spare byte
//
// When S=0 (node-related messages: HEARTBEAT, ASSOCIATION_SETUP):
//   Total header = 8 bytes (no SEID field).
// When S=1 (session-related messages):
//   Total header = 16 bytes (SEID present).
// ---------------------------------------------------------------------------

struct PFCPHeader {
    uint8_t  version        = 1;   // must be 1
    bool     seid_present   = false;
    uint8_t  message_type   = 0;
    uint16_t message_length = 0;   // length after octet 4 (header excl. first 4 bytes + body)
    uint64_t seid           = 0;   // meaningful only when seid_present == true
    uint32_t sequence_number = 0;  // 24-bit on the wire; stored as 32-bit for convenience

    // Size of this header on the wire.
    std::size_t wireSize() const noexcept { return seid_present ? 16u : 8u; }
};

// Encode header to bytes (8 or 16 bytes depending on seid_present).
std::vector<uint8_t> encodePFCPHeader(const PFCPHeader& hdr);

// Decode header from the start of a byte buffer.
// Returns std::nullopt if the buffer is too short or version != 1.
std::optional<PFCPHeader> decodePFCPHeader(const std::vector<uint8_t>& buf);

// ---------------------------------------------------------------------------
// Subset of PFCP IE type codes from TS 29.244 Table 7.5.2-1.
// Values >= 0x8000 are enterprise-specific private extensions.
// ---------------------------------------------------------------------------
enum class IEType : uint16_t {
    CREATE_PDR              = 1,
    PDI                     = 2,
    CREATE_FAR              = 3,
    FORWARDING_PARAMETERS   = 4,
    CREATE_URR              = 6,
    CREATE_QER              = 7,    // Quality Enforcement Rule (TS 29.244 §7.5.2.7)
    UPDATE_PDR              = 9,    // SESSION_MODIFICATION grouped IE
    UPDATE_FAR              = 10,
    UPDATE_URR              = 13,
    UPDATE_QER              = 14,
    REMOVE_PDR              = 15,
    REMOVE_FAR              = 16,
    CAUSE                   = 19,
    SOURCE_INTERFACE        = 20,
    F_TEID                  = 21,
    NETWORK_INSTANCE        = 22,
    GATE_STATUS             = 25,   // QER gate (TS 29.244 §8.2.26)
    MBR                     = 26,   // Maximum Bitrate (TS 29.244 §8.2.6)
    GBR                     = 27,   // Guaranteed Bitrate (TS 29.244 §8.2.7)
    REDIRECT_INFORMATION    = 38,
    OFFENDING_IE            = 40,
    FORWARDING_POLICY       = 41,
    DESTINATION_INTERFACE   = 42,
    UP_FUNCTION_FEATURES    = 43,
    APPLY_ACTION            = 44,
    MEASUREMENT_METHOD      = 62,
    REPORTING_TRIGGERS      = 63,
    MEASUREMENT_PERIOD      = 64,
    F_SEID                  = 57,
    NODE_ID                 = 60,
    UE_IP_ADDRESS           = 93,
    PDR_ID                  = 56,
    VOLUME_THRESHOLD        = 31,
    TIME_THRESHOLD          = 32,
    PRECEDENCE              = 29,
    FAR_ID                  = 108,
    URR_ID                  = 81,
    QER_ID                  = 109,  // TS 29.244 Table 7.5.2-1
    VOLUME_MEASUREMENT      = 66,
    DURATION_MEASUREMENT    = 67,
    START_TIME              = 73,
    END_TIME                = 74,
    USAGE_REPORT_IN_SESSION_MODIFICATION_RESPONSE = 78,
    USAGE_REPORT_IN_SESSION_DELETION_RESPONSE     = 80,
    OUTER_HEADER_CREATION   = 84,
    UR_SEQN                 = 104,

    // Enterprise extension: SUPI of the authenticated subscriber (UTF-8, no NUL terminator).
    AUSF_SUPI               = 0x8001,
};

// ---------------------------------------------------------------------------
// A single PFCP Information Element encoded as TLV
// (2-byte big-endian type + 2-byte big-endian length + value bytes).
// ---------------------------------------------------------------------------
struct InformationElement {
    IEType               type{};
    std::vector<uint8_t> value;

    // Construct an AUSF_SUPI IE from a SUPI string.
    static InformationElement fromSupi(const std::string& supi);

    // Decode value bytes as a UTF-8 SUPI string (meaningful only for AUSF_SUPI IEs).
    std::string toSupiString() const;
};

// Encode a list of IEs into a flat byte buffer.
std::vector<uint8_t> encodeIEs(const std::vector<InformationElement>& ies);

// Decode IEs from a flat byte buffer.
// Truncated IEs (length exceeds remaining bytes) are silently dropped.
std::vector<InformationElement> parseIEs(const std::vector<uint8_t>& payload);

// Return a pointer to the first IE of the given type, or nullptr if absent.
const InformationElement* findIE(const std::vector<InformationElement>& ies, IEType type);

} // namespace networking
} // namespace ausf
