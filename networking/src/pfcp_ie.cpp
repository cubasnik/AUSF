#include "pfcp_ie.h"

namespace ausf {
namespace networking {

// ---------------------------------------------------------------------------
// PFCP header encode / decode  (TS 29.244 §7.2.3)
// ---------------------------------------------------------------------------

std::vector<uint8_t> encodePFCPHeader(const PFCPHeader& hdr) {
    // Octet 1: version (bits 7-5) | spare (bits 4-3) | FO=0 | MP=0 | S
    const uint8_t oct1 = static_cast<uint8_t>(((hdr.version & 0x07u) << 5u) |
                                               (hdr.seid_present ? 0x01u : 0x00u));
    // Message Length = wire bytes after octet 4.
    // If caller has pre-set message_length we honour it; otherwise just encode it.
    const uint16_t msg_len = hdr.message_length;

    std::vector<uint8_t> buf;
    buf.reserve(hdr.wireSize());

    buf.push_back(oct1);
    buf.push_back(hdr.message_type);
    buf.push_back(static_cast<uint8_t>(msg_len >> 8u));
    buf.push_back(static_cast<uint8_t>(msg_len & 0xFFu));

    if (hdr.seid_present) {
        // SEID: 8 bytes big-endian
        for (int i = 7; i >= 0; --i)
            buf.push_back(static_cast<uint8_t>((hdr.seid >> (i * 8u)) & 0xFFu));
    }

    // Sequence number: 3 bytes big-endian + 1 spare byte
    buf.push_back(static_cast<uint8_t>((hdr.sequence_number >> 16u) & 0xFFu));
    buf.push_back(static_cast<uint8_t>((hdr.sequence_number >>  8u) & 0xFFu));
    buf.push_back(static_cast<uint8_t>( hdr.sequence_number         & 0xFFu));
    buf.push_back(0x00u); // spare

    return buf;
}

std::optional<PFCPHeader> decodePFCPHeader(const std::vector<uint8_t>& buf) {
    // Minimum: 8 bytes (no SEID)
    if (buf.size() < 8) return std::nullopt;

    PFCPHeader hdr;
    hdr.version      = (buf[0] >> 5u) & 0x07u;
    if (hdr.version != 1) return std::nullopt;

    hdr.seid_present  = (buf[0] & 0x01u) != 0;
    hdr.message_type  = buf[1];
    hdr.message_length = (static_cast<uint16_t>(buf[2]) << 8u) | buf[3];

    std::size_t offset = 4;

    if (hdr.seid_present) {
        if (buf.size() < 16) return std::nullopt;
        hdr.seid = 0;
        for (int i = 0; i < 8; ++i)
            hdr.seid = (hdr.seid << 8u) | buf[offset + i];
        offset += 8;
    }

    // Sequence number (3 bytes) + spare
    hdr.sequence_number = (static_cast<uint32_t>(buf[offset])     << 16u)
                        | (static_cast<uint32_t>(buf[offset + 1]) <<  8u)
                        |  static_cast<uint32_t>(buf[offset + 2]);

    return hdr;
}

// ---------------------------------------------------------------------------
// IE TLV codec
// ---------------------------------------------------------------------------

InformationElement InformationElement::fromSupi(const std::string& supi) {
    InformationElement ie;
    ie.type  = IEType::AUSF_SUPI;
    ie.value.assign(supi.begin(), supi.end());
    return ie;
}

std::string InformationElement::toSupiString() const {
    return {value.begin(), value.end()};
}

std::vector<uint8_t> encodeIEs(const std::vector<InformationElement>& ies) {
    std::vector<uint8_t> out;
    for (const auto& ie : ies) {
        const auto type_raw = static_cast<uint16_t>(ie.type);
        const auto length   = static_cast<uint16_t>(ie.value.size());
        out.push_back(static_cast<uint8_t>(type_raw >> 8));
        out.push_back(static_cast<uint8_t>(type_raw & 0xFFu));
        out.push_back(static_cast<uint8_t>(length >> 8));
        out.push_back(static_cast<uint8_t>(length & 0xFFu));
        out.insert(out.end(), ie.value.begin(), ie.value.end());
    }
    return out;
}

std::vector<InformationElement> parseIEs(const std::vector<uint8_t>& payload) {
    std::vector<InformationElement> result;
    std::size_t offset = 0;
    while (offset + 4 <= payload.size()) {
        const uint16_t type_raw = (static_cast<uint16_t>(payload[offset])     << 8u)
                                | (static_cast<uint16_t>(payload[offset + 1]));
        const uint16_t length   = (static_cast<uint16_t>(payload[offset + 2]) << 8u)
                                | (static_cast<uint16_t>(payload[offset + 3]));
        offset += 4;
        if (offset + length > payload.size()) break; // truncated IE — stop parsing

        InformationElement ie;
        ie.type  = static_cast<IEType>(type_raw);
        ie.value = {payload.begin() + offset, payload.begin() + offset + length};
        result.push_back(std::move(ie));
        offset += length;
    }
    return result;
}

const InformationElement* findIE(const std::vector<InformationElement>& ies, IEType type) {
    for (const auto& ie : ies) {
        if (ie.type == type) return &ie;
    }
    return nullptr;
}

} // namespace networking
} // namespace ausf
