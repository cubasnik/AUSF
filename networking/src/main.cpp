#include "authentication_result.h"
#include "gtpu.h"
#include "pfcp_data_plane.h"
#include "pfcp_handler.h"
#include "pfcp_ie.h"
#include "pfcp_rules.h"
#include "pfcp_socket.h"

#include <array>
#include <chrono>
#include <cstdint>
#include <iostream>
#include <string>
#include <vector>

using namespace ausf::networking;

// ---------------------------------------------------------------------------
// Minimal test runner — no external dependencies
// ---------------------------------------------------------------------------

static int g_pass    = 0;
static int g_fail    = 0;
static int g_section = 0;

#define SECTION(name) \
    do { ++g_section; std::cout << "\n[" << g_section << "] " << name << "\n"; } while (0)

#define ASSERT_TRUE(cond) \
    do { \
        if (cond) { std::cout << "  PASS  " #cond "\n"; ++g_pass; } \
        else       { std::cerr << "  FAIL  " #cond "\n"; ++g_fail; } \
    } while (0)

#define ASSERT_FALSE(cond) \
    ASSERT_TRUE(!(cond))

#define ASSERT_EQ(a, b) \
    do { \
        if ((a) == (b)) { std::cout << "  PASS  " #a " == " #b "\n"; ++g_pass; } \
        else            { std::cerr << "  FAIL  " #a " [" << (a) << "] != " #b " [" << (b) << "]\n"; ++g_fail; } \
    } while (0)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Build a raw PFCP PDU (header + payload bytes) for use in tests.
// message_length is auto-computed per TS 29.244: bytes after octet 4.
static std::vector<uint8_t> makeRawPDU(PFCPMessageType type,
                                       uint64_t seid,
                                       uint32_t seq,
                                       bool seid_present,
                                       const std::vector<uint8_t>& payload) {
    PFCPHeader h{};
    h.version        = 1;
    h.message_type   = static_cast<uint8_t>(type);
    h.seid           = seid;
    h.seid_present   = seid_present;
    h.sequence_number = seq;
    // message_length = (header bytes after octet 4) + payload
    // S=0: 4 bytes (seq+spare) → 4; S=1: 8+4=12
    h.message_length = static_cast<uint16_t>(
        (seid_present ? 12u : 4u) + payload.size());

    auto pdu = encodePFCPHeader(h);
    pdu.insert(pdu.end(), payload.begin(), payload.end());
    return pdu;
}

// Convenience: node-level PDU (no SEID).
static std::vector<uint8_t> makeNodePDU(PFCPMessageType type, uint32_t seq,
                                         const std::vector<uint8_t>& payload = {}) {
    return makeRawPDU(type, 0, seq, false, payload);
}

// Convenience: session-level PDU (SEID present).
static std::vector<uint8_t> makeSessionPDU(PFCPMessageType type,
                                            uint64_t seid, uint32_t seq,
                                            const std::vector<uint8_t>& payload) {
    return makeRawPDU(type, seid, seq, true, payload);
}

// Build a SESSION_ESTABLISHMENT payload that carries the AUSF_SUPI IE.
static std::vector<uint8_t> buildEstablishPayload(const std::string& supi) {
    std::vector<InformationElement> ies;
    ies.push_back(InformationElement::fromSupi(supi));
    return encodeIEs(ies);
}

// Build an authenticated AuthenticationResult.
static AuthenticationResult makeAuthResult(const std::string& supi,
                                           AuthStatus status = AuthStatus::AUTHENTICATED) {
    AuthenticationResult r;
    r.supi           = supi;
    r.auth_method    = "5G_AKA";
    r.status         = status;
    r.authenticated_at = std::chrono::system_clock::now();
    r.keys.kausf.fill(0xAA);
    r.keys.kseaf.fill(0xBB);
    return r;
}

// Establish association (no socket needed — handleMessage works standalone).
static bool setupHandler(PFCPHandler& h) {
    auto pdu = makeNodePDU(PFCPMessageType::ASSOCIATION_SETUP_REQUEST, 1);
    return h.handleMessage(pdu);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

int main() {
    std::cout << "=== AUSF Networking layer — integration tests ===\n";

    // -----------------------------------------------------------------------
    SECTION("PFCP header encode/decode round-trip S=0 (no SEID)");
    {
        PFCPHeader h{};
        h.version        = 1;
        h.message_type   = 5;   // ASSOCIATION_SETUP_REQUEST
        h.seid_present   = false;
        h.seid           = 0;
        h.sequence_number = 42;
        h.message_length = 4;   // seq(3)+spare(1)

        const auto bytes = encodePFCPHeader(h);
        ASSERT_EQ(bytes.size(), std::size_t(8));

        const auto decoded = decodePFCPHeader(bytes);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->version,         h.version);
        ASSERT_EQ(decoded->seid_present,    h.seid_present);
        ASSERT_EQ(decoded->message_type,    h.message_type);
        ASSERT_EQ(decoded->message_length,  h.message_length);
        ASSERT_EQ(decoded->sequence_number, h.sequence_number);
    }

    // -----------------------------------------------------------------------
    SECTION("PFCP header encode/decode round-trip S=1 (SEID present)");
    {
        PFCPHeader h{};
        h.version        = 1;
        h.message_type   = 50;  // SESSION_ESTABLISHMENT_REQUEST
        h.seid_present   = true;
        h.seid           = 0xDEADBEEF12345678ULL;
        h.sequence_number = 0xABCDEF;
        h.message_length = 12;

        const auto bytes = encodePFCPHeader(h);
        ASSERT_EQ(bytes.size(), std::size_t(16));
        ASSERT_EQ(bytes[0], static_cast<uint8_t>((1 << 5) | 1)); // version=1, S=1

        const auto decoded = decodePFCPHeader(bytes);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->seid,            h.seid);
        ASSERT_EQ(decoded->sequence_number, h.sequence_number);
    }

    // -----------------------------------------------------------------------
    SECTION("PFCP header decode: invalid version rejected");
    {
        std::vector<uint8_t> buf(8, 0x00);
        buf[0] = 0x40u; // version=2, not 1
        ASSERT_FALSE(decodePFCPHeader(buf).has_value());
    }

    // -----------------------------------------------------------------------
    SECTION("PFCP header decode: truncated buffer");
    {
        std::vector<uint8_t> buf(6, 0x20); // version=1, S=0 but only 6 bytes
        ASSERT_FALSE(decodePFCPHeader(buf).has_value());
    }

    // -----------------------------------------------------------------------
    SECTION("PFCP header decode: S=1 but too short for SEID");
    {
        std::vector<uint8_t> buf(12, 0x00);
        buf[0] = static_cast<uint8_t>((1u << 5u) | 1u); // version=1, S=1
        // buf is only 12 bytes; need 16 for SEID
        ASSERT_FALSE(decodePFCPHeader(buf).has_value());
    }

    // -----------------------------------------------------------------------
    SECTION("PFCP IE encode / parse round-trip");
    {
        const std::string supi = "imsi-250010000000001";
        auto ie  = InformationElement::fromSupi(supi);
        auto buf = encodeIEs({ie});
        auto ies = parseIEs(buf);

        ASSERT_EQ(ies.size(), std::size_t(1));
        ASSERT_EQ(ies[0].toSupiString(), supi);
        ASSERT_TRUE(findIE(ies, IEType::AUSF_SUPI) != nullptr);
        ASSERT_TRUE(findIE(ies, IEType::NODE_ID)   == nullptr);
    }

    // -----------------------------------------------------------------------
    SECTION("Truncated IE payload is handled safely");
    {
        std::vector<uint8_t> bad = {0x80, 0x01, 0x00, 100, 0xAB, 0xCD};
        auto ies = parseIEs(bad);
        ASSERT_TRUE(ies.empty());
    }

    // -----------------------------------------------------------------------
    SECTION("AuthenticationResult statusString");
    {
        ASSERT_EQ(makeAuthResult("x", AuthStatus::AUTHENTICATED).statusString(),
                  std::string("AUTHENTICATED"));
        ASSERT_EQ(makeAuthResult("x", AuthStatus::REJECTED).statusString(),
                  std::string("REJECTED"));
        ASSERT_EQ(makeAuthResult("x", AuthStatus::PENDING).statusString(),
                  std::string("PENDING"));
    }

    // -----------------------------------------------------------------------
    SECTION("PDRRule encode/decode round-trip");
    {
        PDRRule pdr;
        pdr.pdr_id     = 1;
        pdr.precedence = 100;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid = FTEID::withIPv4(0x0001, 0xC0A80101); // 192.168.1.1
        pdr.far_id     = 5;

        const auto ies = pdr.toIEs();
        const auto decoded = PDRRule::fromIEs(ies);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->pdr_id,     pdr.pdr_id);
        ASSERT_EQ(decoded->precedence, pdr.precedence);
        ASSERT_EQ(decoded->far_id,     pdr.far_id);
        {
            const int got  = static_cast<int>(decoded->pdi.source_interface);
            const int want = static_cast<int>(InterfaceType::ACCESS);
            ASSERT_EQ(got, want);
            ASSERT_TRUE(decoded->pdi.local_fteid.has_value());
            const uint32_t got_teid  = decoded->pdi.local_fteid->teid;
            const uint32_t want_teid = pdr.pdi.local_fteid->teid;
            ASSERT_EQ(got_teid, want_teid);
        }
    }

    // -----------------------------------------------------------------------
    SECTION("FARRule encode/decode round-trip (FORW with forwarding params)");
    {
        FARRule rule;
        rule.far_id              = 5;
        rule.apply_action.forw   = true;
        ForwardingParameters fp;
        fp.destination_interface = InterfaceType::CORE;
        fp.network_instance      = "internet";
        rule.forwarding_parameters = fp;

        const auto ies = rule.toIEs();
        const auto decoded = FARRule::fromIEs(ies);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->far_id, rule.far_id);
        ASSERT_TRUE(decoded->apply_action.forw);
        ASSERT_FALSE(decoded->apply_action.drop);
        ASSERT_TRUE(decoded->forwarding_parameters.has_value());
        {
            const std::string got_ni = decoded->forwarding_parameters->network_instance;
            ASSERT_EQ(got_ni, std::string("internet"));
        }
    }

    // -----------------------------------------------------------------------
    SECTION("URRRule encode/decode round-trip");
    {
        URRRule urr;
        urr.urr_id                        = 1;
        urr.measurement_method.volume      = true;
        urr.reporting_triggers.perio       = true;
        urr.measurement_period             = 3600;

        const auto ies = urr.toIEs();
        const auto decoded = URRRule::fromIEs(ies);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->urr_id,              urr.urr_id);
        ASSERT_TRUE(decoded->measurement_method.volume);
        ASSERT_FALSE(decoded->measurement_method.durat);
        ASSERT_TRUE(decoded->reporting_triggers.perio);
        ASSERT_EQ(decoded->measurement_period, 3600u);
    }

    // -----------------------------------------------------------------------
    SECTION("Handler: association setup");
    {
        PFCPHandler h(8805);
        ASSERT_FALSE(h.isAssociationEstablished());
        ASSERT_TRUE(h.handleMessage(
            makeNodePDU(PFCPMessageType::ASSOCIATION_SETUP_REQUEST, 1)));
        ASSERT_TRUE(h.isAssociationEstablished());
    }

    // -----------------------------------------------------------------------
    SECTION("Heartbeat rejected before association");
    {
        PFCPHandler h(8805);
        ASSERT_FALSE(h.handleMessage(
            makeNodePDU(PFCPMessageType::HEARTBEAT_REQUEST, 1)));
    }

    // -----------------------------------------------------------------------
    SECTION("Session establishment rejected: association not established");
    {
        PFCPHandler h(8805);
        const std::string supi = "imsi-250010000000042";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        const auto payload = buildEstablishPayload(supi);
        ASSERT_FALSE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           1001, 2, payload)));
        ASSERT_EQ(h.getSessionCount(), std::size_t(0));
    }

    // -----------------------------------------------------------------------
    SECTION("Session establishment rejected: AUSF_SUPI IE missing");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        ASSERT_FALSE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           2001, 3, {})));
        ASSERT_EQ(h.getSessionCount(), std::size_t(0));
    }

    // -----------------------------------------------------------------------
    SECTION("Session establishment rejected: no auth result on record");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const auto payload = buildEstablishPayload("imsi-250010000000099");
        ASSERT_FALSE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           3001, 4, payload)));
        ASSERT_EQ(h.getSessionCount(), std::size_t(0));
    }

    // -----------------------------------------------------------------------
    SECTION("Session establishment rejected: auth status REJECTED");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000011";
        h.notifyAuthenticationResult(makeAuthResult(supi, AuthStatus::REJECTED));
        const auto payload = buildEstablishPayload(supi);
        ASSERT_FALSE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           4001, 5, payload)));
        ASSERT_EQ(h.getSessionCount(), std::size_t(0));
    }

    // -----------------------------------------------------------------------
    SECTION("Session establishment rejected: auth status PENDING");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000012";
        h.notifyAuthenticationResult(makeAuthResult(supi, AuthStatus::PENDING));
        const auto payload = buildEstablishPayload(supi);
        ASSERT_FALSE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           5001, 6, payload)));
        ASSERT_EQ(h.getSessionCount(), std::size_t(0));
    }

    // -----------------------------------------------------------------------
    SECTION("Session established for authenticated subscriber");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000001";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        ASSERT_TRUE(h.getAuthResult(supi).has_value());
        ASSERT_TRUE(h.getAuthResult(supi)->isAuthenticated());

        const auto payload = buildEstablishPayload(supi);
        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           6001, 7, payload)));
        ASSERT_EQ(h.getSessionCount(), std::size_t(1));
    }

    // -----------------------------------------------------------------------
    SECTION("Session modification and deletion lifecycle");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000002";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        const uint64_t seid = 9000;
        const auto ep = buildEstablishPayload(supi);
        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST, seid, 10, ep)));
        ASSERT_EQ(h.getSessionCount(), std::size_t(1));

        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_MODIFICATION_REQUEST, seid, 11, {})));

        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_DELETION_REQUEST, seid, 12, {})));
        ASSERT_EQ(h.getSessionCount(), std::size_t(0));
    }

    // -----------------------------------------------------------------------
    SECTION("Multiple authenticated subscribers — independent sessions");
    {
        PFCPHandler h(8805);
        setupHandler(h);

        const std::string supi1 = "imsi-250010000000010";
        const std::string supi2 = "imsi-250010000000011";
        h.notifyAuthenticationResult(makeAuthResult(supi1));
        h.notifyAuthenticationResult(makeAuthResult(supi2));

        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           7001, 20, buildEstablishPayload(supi1))));
        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           7002, 21, buildEstablishPayload(supi2))));
        ASSERT_EQ(h.getSessionCount(), std::size_t(2));

        // Unauthenticated third subscriber must be rejected.
        ASSERT_FALSE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           7003, 22, buildEstablishPayload("imsi-250010000000099"))));
        ASSERT_EQ(h.getSessionCount(), std::size_t(2));
    }

    // -----------------------------------------------------------------------
    SECTION("getAuthResult returns nullopt for unknown SUPI");
    {
        PFCPHandler h(8805);
        ASSERT_FALSE(h.getAuthResult("imsi-000000000000000").has_value());
    }

    // =======================================================================
    // GTP-U codec tests (23–28)
    // =======================================================================

    SECTION("GTP-U: G-PDU header encode/decode (no optional fields)");
    {
        GTPUHeader hdr{};
        hdr.version      = 1;
        hdr.pt           = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid         = 0xDEADBEEFu;
        const auto enc = encodeGTPUHeader(hdr);
        ASSERT_EQ(enc.size(), std::size_t(8));
        ASSERT_EQ(enc[0], 0x30u);               // version=1, PT=1, E=0, S=0, PN=0
        ASSERT_EQ(enc[4], 0xDEu);               // TEID MSB
        const auto dec = decodeGTPUHeader(enc);
        ASSERT_TRUE(dec.has_value());
        ASSERT_EQ(dec->teid, 0xDEADBEEFu);
        ASSERT_TRUE(dec->message_type == GTPUMessageType::G_PDU);
        ASSERT_EQ(dec->wireSize(), std::size_t(8));
    }

    SECTION("GTP-U: header with sequence number (S=1)");
    {
        GTPUHeader hdr{};
        hdr.version          = 1;
        hdr.pt               = true;
        hdr.s                = true;
        hdr.message_type     = GTPUMessageType::G_PDU;
        hdr.teid             = 0x00000042u;
        hdr.sequence_number  = 0x1234u;
        const auto enc = encodeGTPUHeader(hdr);
        ASSERT_EQ(enc.size(), std::size_t(12));
        ASSERT_EQ(enc[0] & 0x02u, 0x02u);       // S flag
        const auto dec = decodeGTPUHeader(enc);
        ASSERT_TRUE(dec.has_value());
        ASSERT_EQ(dec->sequence_number, uint16_t(0x1234));
        ASSERT_EQ(dec->wireSize(), std::size_t(12));
    }

    SECTION("GTP-U: G-PDU frame with payload round-trip");
    {
        GTPUHeader hdr{};
        hdr.version      = 1;
        hdr.pt           = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid         = 0x0000000Au;
        const std::vector<uint8_t> payload = {0x45, 0x00, 0x00, 0x28};
        const auto frame = encodeGTPU(hdr, payload);
        ASSERT_EQ(frame.size(), std::size_t(8 + 4));  // header + 4-byte payload
        const auto decoded = decodeGTPU(frame);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->header.teid, 0x0000000Au);
        ASSERT_TRUE(decoded->payload == payload);
    }

    SECTION("GTP-U: invalid version is rejected");
    {
        GTPUHeader hdr{};
        hdr.version      = 0;               // wrong — must be 1
        hdr.pt           = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid         = 0x00000001u;
        const auto enc = encodeGTPUHeader(hdr);
        const auto dec = decodeGTPUHeader(enc);
        ASSERT_FALSE(dec.has_value());
    }

    SECTION("GTP-U: truncated header (<8 bytes) is rejected");
    {
        const std::vector<uint8_t> short_buf = {0x20, 0xFF, 0x00};
        const auto dec = decodeGTPUHeader(short_buf);
        ASSERT_FALSE(dec.has_value());
    }

    SECTION("GTP-U: S=1 but buffer too short for optional fields is rejected");
    {
        // Build a valid 12-byte frame, then truncate to 10.
        GTPUHeader hdr{};
        hdr.version      = 1;
        hdr.pt           = true;
        hdr.s            = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid         = 0x00000001u;
        auto enc = encodeGTPUHeader(hdr);
        ASSERT_EQ(enc.size(), std::size_t(12));
        enc.resize(10);                         // truncate
        const auto dec = decodeGTPUHeader(enc);
        ASSERT_FALSE(dec.has_value());
    }

    // =======================================================================
    // VolumeMeasurement + UsageReport tests (29–30)
    // =======================================================================

    SECTION("VolumeMeasurement: encode/decode round-trip (UL+DL+total)");
    {
        const auto vm = VolumeMeasurement::withULDL(1000u, 2000u);
        ASSERT_TRUE(vm.ul_present);
        ASSERT_TRUE(vm.dl_present);
        ASSERT_TRUE(vm.total_present);
        ASSERT_EQ(vm.ul_octets,    uint64_t(1000));
        ASSERT_EQ(vm.dl_octets,    uint64_t(2000));
        ASSERT_EQ(vm.total_octets, uint64_t(3000));

        const auto encoded = vm.encode();
        // flags(1) + total(8) + ul(8) + dl(8) = 25 bytes
        ASSERT_EQ(encoded.size(), std::size_t(25));

        const auto decoded = VolumeMeasurement::decode(encoded);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->ul_octets,    uint64_t(1000));
        ASSERT_EQ(decoded->dl_octets,    uint64_t(2000));
        ASSERT_EQ(decoded->total_octets, uint64_t(3000));
    }

    SECTION("UsageReport: toIEs produces 4 IEs with correct types");
    {
        UsageReport rep;
        rep.urr_id  = 7u;
        rep.ur_seqn = 3u;
        rep.trigger.perio = true;
        rep.volume  = VolumeMeasurement::withULDL(500u, 800u);
        const auto ies = rep.toIEs();
        ASSERT_EQ(ies.size(), std::size_t(4));
        ASSERT_TRUE(ies[0].type == IEType::URR_ID);
        ASSERT_TRUE(ies[1].type == IEType::UR_SEQN);
        ASSERT_TRUE(ies[2].type == IEType::REPORTING_TRIGGERS);
        ASSERT_TRUE(ies[3].type == IEType::VOLUME_MEASUREMENT);
        // URR_ID value = big-endian 7
        ASSERT_EQ(ies[0].value[3], uint8_t(7));
    }

    // =======================================================================
    // Rule installation + usage accounting tests (31–33)
    // =======================================================================

    // Helper: build SESSION_ESTABLISHMENT payload that also carries
    // a CREATE_PDR, CREATE_FAR, and CREATE_URR grouped IE.
    auto buildEstablishPayloadWithRules = [&](const std::string& supi,
                                              uint16_t pdr_id_val,
                                              uint32_t far_id_val,
                                              uint32_t urr_id_val) {
        // Minimal PDRRule
        PDRRule pdr{};
        pdr.pdr_id     = pdr_id_val;
        pdr.precedence = 100;
        pdr.far_id     = far_id_val;
        pdr.pdi.source_interface = InterfaceType::ACCESS;

        // Minimal FARRule (forward)
        FARRule fr{};
        fr.far_id = far_id_val;
        fr.apply_action.forw = true;

        // Minimal URRRule (periodic + volume)
        URRRule urr{};
        urr.urr_id = urr_id_val;
        urr.measurement_method.volume = true;
        urr.reporting_triggers.perio  = true;
        urr.measurement_period        = 3600u;

        // Wrap each in a grouped IE
        auto wrap = [](IEType grouped_type, const std::vector<InformationElement>& inner) {
            InformationElement g{};
            g.type  = grouped_type;
            g.value = encodeIEs(inner);
            return g;
        };

        std::vector<InformationElement> outer;
        // AUSF_SUPI
        InformationElement supi_ie{};
        supi_ie.type  = IEType::AUSF_SUPI;
        supi_ie.value.assign(supi.begin(), supi.end());
        outer.push_back(supi_ie);

        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_URR, urr.toIEs()));

        return encodeIEs(outer);
    };

    SECTION("SESSION_ESTABLISHMENT installs PDR, FAR, URR rules");
    {
        PFCPHandler h(8805);
        ASSERT_TRUE(h.handleMessage(makeNodePDU(PFCPMessageType::ASSOCIATION_SETUP_REQUEST, 30)));
        const std::string supi = "imsi-250010000000031";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        const uint64_t seid    = 3100;
        const uint32_t seqn    = 31;
        auto payload = buildEstablishPayloadWithRules(supi, 1, 1001, 2001);
        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           seid, seqn, payload)));

        auto session = h.getSession(seid);
        ASSERT_TRUE(session != nullptr);
        ASSERT_EQ(session->getPDRs().size(), std::size_t(1));
        ASSERT_EQ(session->getFARs().size(), std::size_t(1));
        ASSERT_EQ(session->getURRs().size(), std::size_t(1));
        ASSERT_EQ(session->getPDRs()[0].pdr_id, uint16_t(1));
        ASSERT_EQ(session->getFARs()[0].far_id, uint32_t(1001));
        ASSERT_EQ(session->getURRs()[0].urr_id, uint32_t(2001));
    }

    SECTION("Usage counters: accountUplink/Downlink + getters");
    {
        PFCPHandler h(8805);
        ASSERT_TRUE(h.handleMessage(makeNodePDU(PFCPMessageType::ASSOCIATION_SETUP_REQUEST, 320)));
        const std::string supi = "imsi-250010000000032";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        const uint64_t seid = 3200;
        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           seid, 32, buildEstablishPayload(supi))));

        auto session = h.getSession(seid);
        ASSERT_TRUE(session != nullptr);

        ASSERT_EQ(session->getUplinkOctets(),   uint64_t(0));
        ASSERT_EQ(session->getDownlinkOctets(), uint64_t(0));

        session->accountUplinkOctets(1500u, 2u);
        session->accountDownlinkOctets(3000u, 4u);

        ASSERT_EQ(session->getUplinkOctets(),   uint64_t(1500));
        ASSERT_EQ(session->getDownlinkOctets(), uint64_t(3000));
        ASSERT_EQ(session->getUplinkPackets(),  uint64_t(2));
        ASSERT_EQ(session->getDownlinkPackets(), uint64_t(4));
    }

    SECTION("collectUsageReports: periodic + UR-SEQN increment");
    {
        PFCPHandler h(8805);
        ASSERT_TRUE(h.handleMessage(makeNodePDU(PFCPMessageType::ASSOCIATION_SETUP_REQUEST, 330)));
        const std::string supi = "imsi-250010000000033";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        const uint64_t seid = 3300;
        auto payload = buildEstablishPayloadWithRules(supi, 1, 1001, 5001);
        ASSERT_TRUE(h.handleMessage(
            makeSessionPDU(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
                           seid, 33, payload)));

        auto session = h.getSession(seid);
        ASSERT_TRUE(session != nullptr);

        session->accountUplinkOctets(100u);
        session->accountDownlinkOctets(200u);

        // First periodic collection
        const auto r1 = session->collectUsageReports(true);
        ASSERT_EQ(r1.size(), std::size_t(1));
        ASSERT_EQ(r1[0].urr_id,  uint32_t(5001));
        ASSERT_EQ(r1[0].ur_seqn, uint32_t(1));
        ASSERT_TRUE(r1[0].trigger.perio);
        ASSERT_EQ(r1[0].volume.ul_octets, uint64_t(100));
        ASSERT_EQ(r1[0].volume.dl_octets, uint64_t(200));

        // Second call increments sequence number
        const auto r2 = session->collectUsageReports(true);
        ASSERT_EQ(r2[0].ur_seqn, uint32_t(2));

        // Non-periodic collection includes the same URR
        const auto r3 = session->collectUsageReports(false);
        ASSERT_EQ(r3.size(), std::size_t(1));
        ASSERT_EQ(r3[0].ur_seqn, uint32_t(3));
    }

    // =======================================================================
    // URRRule volume_threshold encode/decode round-trip (34)
    // =======================================================================

    SECTION("URRRule with volume_threshold encode/decode round-trip");
    {
        URRRule urr{};
        urr.urr_id = 99u;
        urr.measurement_method.volume      = true;
        urr.reporting_triggers.volth       = true;
        urr.volume_threshold               = VolumeMeasurement::withULDL(500'000u, 500'000u);

        const auto ies      = urr.toIEs();
        const auto decoded  = URRRule::fromIEs(ies);
        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->urr_id, uint32_t(99));
        ASSERT_TRUE(decoded->reporting_triggers.volth);
        ASSERT_TRUE(decoded->volume_threshold.has_value());
        ASSERT_EQ(decoded->volume_threshold->ul_octets, uint64_t(500'000));
        ASSERT_EQ(decoded->volume_threshold->total_octets, uint64_t(1'000'000));
    }

    // =======================================================================
    // DataPlane tests (35–40)
    // =======================================================================

    // Helper: build a session with a FORW PDR (access→core) and a matching FAR.
    auto makeDPSession = [&](PFCPHandler& h,
                              uint64_t seid, uint32_t seq_base,
                              const std::string& supi,
                              uint32_t local_teid,
                              uint32_t remote_teid,
                              bool drop = false) -> std::shared_ptr<PFCPSession> {
        // PDR: ACCESS interface, local F-TEID = local_teid
        PDRRule pdr{};
        pdr.pdr_id     = 1;
        pdr.precedence = 100;
        pdr.far_id     = 101;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(local_teid, 0x7f000001u);

        // FAR: FORW (or DROP) to remote_teid on core side
        FARRule fr{};
        fr.far_id = 101;
        if (drop) {
            fr.apply_action.drop = true;
        } else {
            fr.apply_action.forw = true;
            ForwardingParameters fp{};
            fp.destination_interface   = InterfaceType::CORE;
            fp.outer_header_creation   = FTEID::withIPv4(remote_teid, 0x0a000001u);
            fr.forwarding_parameters   = fp;
        }

        // Build SESSION_ESTABLISHMENT payload with PDR+FAR.
        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{};
            g.type  = t;
            g.value = encodeIEs(inner);
            return g;
        };

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            seid, seq_base, encodeIEs(outer)));
        return h.getSession(seid);
    };

    SECTION("DataPlane: addSession builds TEID index");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000035";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        auto session = makeDPSession(h, 3500, 350, supi, 0xAAAA0001u, 0xBBBB0001u);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);
        ASSERT_EQ(dp.sessionCount(), std::size_t(1));

        dp.removeSession(session->getSEID());
        ASSERT_EQ(dp.sessionCount(), std::size_t(0));
    }

    SECTION("DataPlane: processUplink FORW — frame produced, counters updated");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000036";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        auto session = makeDPSession(h, 3600, 360, supi, 0x00001234u, 0x00005678u);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        // Build incoming G-PDU with TEID=0x1234 and 20-byte IP payload.
        GTPUHeader in_hdr{};
        in_hdr.version      = 1;
        in_hdr.pt           = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid         = 0x00001234u;
        const std::vector<uint8_t> ip_pkt(20, 0x45u); // dummy IP header bytes
        const auto raw_frame = encodeGTPU(in_hdr, ip_pkt);
        const auto decoded_frame = decodeGTPU(raw_frame);
        ASSERT_TRUE(decoded_frame.has_value());

        const auto result = dp.processUplink(*decoded_frame);

        ASSERT_TRUE(result.forwarded);
        ASSERT_FALSE(result.dropped);
        ASSERT_EQ(result.session_seid, uint64_t(3600));

        // Outgoing frame must use far's remote TEID (0x5678)
        const auto out_frame = decodeGTPU(result.frame);
        ASSERT_TRUE(out_frame.has_value());
        ASSERT_EQ(out_frame->header.teid, uint32_t(0x00005678u));
        ASSERT_TRUE(out_frame->payload == ip_pkt);  // payload preserved

        // UL counter must reflect the payload size
        ASSERT_EQ(session->getUplinkOctets(), uint64_t(20));
        ASSERT_EQ(session->getUplinkPackets(), uint64_t(1));
    }

    SECTION("DataPlane: processUplink DROP — no frame, counters updated");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000037";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        auto session = makeDPSession(h, 3700, 370, supi, 0xDEAD0001u, 0u, /*drop=*/true);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        GTPUHeader in_hdr{};
        in_hdr.version      = 1;
        in_hdr.pt           = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid         = 0xDEAD0001u;
        const std::vector<uint8_t> pkt(40, 0x00u);
        const auto raw = encodeGTPU(in_hdr, pkt);
        const auto frm = decodeGTPU(raw);
        ASSERT_TRUE(frm.has_value());

        const auto result = dp.processUplink(*frm);

        ASSERT_FALSE(result.forwarded);
        ASSERT_TRUE(result.dropped);
        ASSERT_TRUE(result.frame.empty());
        ASSERT_EQ(session->getUplinkOctets(), uint64_t(40));
    }

    SECTION("DataPlane: processUplink with unknown TEID — no crash, no match");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000038";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        auto session = makeDPSession(h, 3800, 380, supi, 0x11110001u, 0x22220001u);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        GTPUHeader in_hdr{};
        in_hdr.version      = 1;
        in_hdr.pt           = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid         = 0xDEADBEEFu; // unknown TEID
        const auto raw = encodeGTPU(in_hdr, {0x45u});
        const auto frm = decodeGTPU(raw);
        ASSERT_TRUE(frm.has_value());

        const auto result = dp.processUplink(*frm);

        ASSERT_FALSE(result.forwarded);
        ASSERT_FALSE(result.dropped);
        ASSERT_EQ(result.session_seid, uint64_t(0)); // no match
        // counters must remain zero
        ASSERT_EQ(session->getUplinkOctets(), uint64_t(0));
    }

    SECTION("DataPlane: FORW with no outer_header_creation preserves TEID");
    {
        // FAR without ForwardingParameters → outgoing TEID = incoming TEID
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000039";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        PDRRule pdr{};
        pdr.pdr_id     = 1;
        pdr.precedence = 100;
        pdr.far_id     = 201;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(0xCAFEu, 0x7f000001u);

        FARRule fr{};
        fr.far_id = 201;
        fr.apply_action.forw = true;
        // No forwarding_parameters → transparent

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };
        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));
        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            3900, 390, encodeIEs(outer)));

        auto session = h.getSession(3900);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        GTPUHeader in_hdr{};
        in_hdr.version      = 1;
        in_hdr.pt           = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid         = 0xCAFEu;
        const auto raw = encodeGTPU(in_hdr, {0x01u, 0x02u});
        const auto frm = decodeGTPU(raw);
        ASSERT_TRUE(frm.has_value());

        const auto result = dp.processUplink(*frm);
        ASSERT_TRUE(result.forwarded);
        const auto out = decodeGTPU(result.frame);
        ASSERT_TRUE(out.has_value());
        ASSERT_EQ(out->header.teid, uint32_t(0xCAFEu)); // preserved
    }

    SECTION("DataPlane: multiple sessions, each gets own TEID bucket");
    {
        PFCPHandler h(8805);
        setupHandler(h);

        const std::string supi1 = "imsi-250010000000040";
        const std::string supi2 = "imsi-250010000000041";
        h.notifyAuthenticationResult(makeAuthResult(supi1));
        h.notifyAuthenticationResult(makeAuthResult(supi2));

        auto s1 = makeDPSession(h, 4000, 400, supi1, 0xAABB0001u, 0xCCDD0001u);
        auto s2 = makeDPSession(h, 4001, 401, supi2, 0xAABB0002u, 0xCCDD0002u);
        ASSERT_TRUE(s1 != nullptr);
        ASSERT_TRUE(s2 != nullptr);

        DataPlane dp;
        dp.addSession(s1);
        dp.addSession(s2);
        ASSERT_EQ(dp.sessionCount(), std::size_t(2));

        // Packet to session 1 TEID
        GTPUHeader h1{};
        h1.version = 1; h1.pt = true;
        h1.message_type = GTPUMessageType::G_PDU;
        h1.teid = 0xAABB0001u;
        const auto f1 = decodeGTPU(encodeGTPU(h1, {0x01u}));
        const auto r1 = dp.processUplink(*f1);
        ASSERT_EQ(r1.session_seid, uint64_t(4000));

        // Packet to session 2 TEID
        GTPUHeader h2{};
        h2.version = 1; h2.pt = true;
        h2.message_type = GTPUMessageType::G_PDU;
        h2.teid = 0xAABB0002u;
        const auto f2 = decodeGTPU(encodeGTPU(h2, {0x02u}));
        const auto r2 = dp.processUplink(*f2);
        ASSERT_EQ(r2.session_seid, uint64_t(4001));
    }

    // =======================================================================
    // Volume threshold triggering tests (41–43)
    // =======================================================================

    // Helper: build SESSION_ESTABLISHMENT payload with PDR+FAR+URR(volth).
    auto buildVolthPayload = [&](const std::string& supi,
                                  uint32_t local_teid,
                                  uint64_t threshold_octets) {
        PDRRule pdr{};
        pdr.pdr_id = 1; pdr.precedence = 100; pdr.far_id = 501;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(local_teid, 0x7f000001u);

        FARRule fr{};
        fr.far_id = 501; fr.apply_action.forw = true;

        URRRule urr{};
        urr.urr_id = 301;
        urr.measurement_method.volume = true;
        urr.reporting_triggers.volth  = true;
        urr.volume_threshold = VolumeMeasurement::withULDL(
            threshold_octets / 2, threshold_octets / 2);

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };
        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_URR, urr.toIEs()));
        return encodeIEs(outer);
    };

    SECTION("Volume threshold: no report below threshold");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000041";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            4100, 410, buildVolthPayload(supi, 0xF0010001u, 10'000u)));

        auto session = h.getSession(4100);
        ASSERT_TRUE(session != nullptr);
        ASSERT_EQ(session->getURRs().size(), std::size_t(1));
        ASSERT_TRUE(session->getURRs()[0].volume_threshold.has_value());

        // Below threshold
        session->accountUplinkOctets(4'999u);

        const auto reports = session->checkVolumeThresholds();
        ASSERT_TRUE(reports.empty());
    }

    SECTION("Volume threshold: report fires when threshold crossed");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000042";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            4200, 420, buildVolthPayload(supi, 0xF0020001u, 10'000u)));

        auto session = h.getSession(4200);
        ASSERT_TRUE(session != nullptr);

        // Cross 10 000-byte threshold
        session->accountUplinkOctets(6'000u);
        session->accountDownlinkOctets(5'000u); // total = 11 000 > 10 000

        const auto reports = session->checkVolumeThresholds();
        ASSERT_EQ(reports.size(), std::size_t(1));
        ASSERT_EQ(reports[0].urr_id, uint32_t(301));
        ASSERT_TRUE(reports[0].trigger.volth);
        ASSERT_EQ(reports[0].ur_seqn, uint32_t(1));
    }

    SECTION("Volume threshold: second crossing fires second report");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000043";
        h.notifyAuthenticationResult(makeAuthResult(supi));
        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            4300, 430, buildVolthPayload(supi, 0xF0030001u, 1'000u)));

        auto session = h.getSession(4300);
        ASSERT_TRUE(session != nullptr);

        // First crossing: 1200 > 1000
        session->accountUplinkOctets(700u);
        session->accountDownlinkOctets(500u); // total=1200

        const auto r1 = session->checkVolumeThresholds();
        ASSERT_EQ(r1.size(), std::size_t(1));
        ASSERT_EQ(r1[0].ur_seqn, uint32_t(1));

        // Check again with same counter — no new report
        const auto r_same = session->checkVolumeThresholds();
        ASSERT_TRUE(r_same.empty());

        // Second crossing: add enough to cross 2000
        session->accountUplinkOctets(900u); // total=2100
        const auto r2 = session->checkVolumeThresholds();
        ASSERT_EQ(r2.size(), std::size_t(1));
        ASSERT_EQ(r2[0].ur_seqn, uint32_t(2));
    }

    // =========================================================================
    // Group 7 — QERRule: Quality Enforcement (gate + MBR/GBR)
    // =========================================================================

    SECTION("QERRule encode/decode round-trip: both gates OPEN, no MBR/GBR");
    {
        QERRule qer{};
        qer.qer_id  = 77;
        qer.ul_gate = GateStatus::OPEN;
        qer.dl_gate = GateStatus::OPEN;

        const auto ies     = qer.toIEs();
        const auto decoded = QERRule::fromIEs(ies);

        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->qer_id,  uint32_t(77));
        ASSERT_TRUE(decoded->ul_gate == GateStatus::OPEN);
        ASSERT_TRUE(decoded->dl_gate == GateStatus::OPEN);
        ASSERT_EQ(decoded->ul_mbr,  uint64_t(0));
        ASSERT_EQ(decoded->dl_mbr,  uint64_t(0));
    }

    SECTION("QERRule encode/decode: UL gate CLOSED, DL gate OPEN + MBR");
    {
        QERRule qer{};
        qer.qer_id  = 88;
        qer.ul_gate = GateStatus::CLOSED;
        qer.dl_gate = GateStatus::OPEN;
        qer.ul_mbr  = 100000u;   // 100 Mbps in Kbps
        qer.dl_mbr  = 200000u;

        const auto ies     = qer.toIEs();
        const auto decoded = QERRule::fromIEs(ies);

        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->qer_id,  uint32_t(88));
        ASSERT_TRUE(decoded->ul_gate == GateStatus::CLOSED);
        ASSERT_TRUE(decoded->dl_gate == GateStatus::OPEN);
        ASSERT_EQ(decoded->ul_mbr,  uint64_t(100000));
        ASSERT_EQ(decoded->dl_mbr,  uint64_t(200000));
        ASSERT_TRUE(decoded->isDownlinkOpen());
        ASSERT_FALSE(decoded->isUplinkOpen());
    }

    SECTION("QERRule encode/decode: both gates CLOSED + GBR");
    {
        QERRule qer{};
        qer.qer_id  = 55;
        qer.ul_gate = GateStatus::CLOSED;
        qer.dl_gate = GateStatus::CLOSED;
        qer.ul_gbr  = 50000u;
        qer.dl_gbr  = 50000u;

        const auto decoded = QERRule::fromIEs(qer.toIEs());

        ASSERT_TRUE(decoded.has_value());
        ASSERT_TRUE(decoded->ul_gate == GateStatus::CLOSED);
        ASSERT_TRUE(decoded->dl_gate == GateStatus::CLOSED);
        ASSERT_EQ(decoded->ul_gbr,  uint64_t(50000));
        ASSERT_EQ(decoded->dl_gbr,  uint64_t(50000));
    }

    SECTION("PDRRule with qer_id encode/decode round-trip");
    {
        PDRRule pdr{};
        pdr.pdr_id     = 5;
        pdr.precedence = 200;
        pdr.far_id     = 99;
        pdr.qer_id     = 77;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(0xABCD1234u, 0x7f000001u);

        const auto decoded = PDRRule::fromIEs(pdr.toIEs());

        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->pdr_id,     uint16_t(5));
        ASSERT_EQ(decoded->qer_id,     uint32_t(77));
        ASSERT_EQ(decoded->far_id,     uint32_t(99));
        ASSERT_EQ(decoded->precedence, uint32_t(200));
    }

    SECTION("PDRRule without qer_id: qer_id encodes as absent, decodes as 0");
    {
        PDRRule pdr{};
        pdr.pdr_id     = 6;
        pdr.precedence = 100;
        pdr.far_id     = 1;
        pdr.qer_id     = 0;   // not set
        pdr.pdi.source_interface = InterfaceType::ACCESS;

        const auto decoded = PDRRule::fromIEs(pdr.toIEs());

        ASSERT_TRUE(decoded.has_value());
        ASSERT_EQ(decoded->qer_id, uint32_t(0));
    }

    // =========================================================================
    // Group 8 — DataPlane: QER gate enforcement
    // =========================================================================

    SECTION("DataPlane: UL gate OPEN → packet forwarded normally");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000055";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        // Build session with PDR referencing QER(ul_gate=OPEN)
        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        PDRRule pdr{};
        pdr.pdr_id = 1; pdr.precedence = 100; pdr.far_id = 101; pdr.qer_id = 10;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(0xFACE0001u, 0x7f000001u);

        FARRule fr{}; fr.far_id = 101; fr.apply_action.forw = true;
        ForwardingParameters fp{};
        fp.destination_interface = InterfaceType::CORE;
        fp.outer_header_creation = FTEID::withIPv4(0xFACE0002u, 0x0a000001u);
        fr.forwarding_parameters = fp;

        QERRule qer{};
        qer.qer_id  = 10;
        qer.ul_gate = GateStatus::OPEN;
        qer.dl_gate = GateStatus::OPEN;

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_QER, qer.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            5500, 550, encodeIEs(outer)));

        auto session = h.getSession(5500);
        ASSERT_TRUE(session != nullptr);
        ASSERT_EQ(session->getQERs().size(), std::size_t(1));

        DataPlane dp;
        dp.addSession(session);

        GTPUHeader in_hdr{}; in_hdr.version = 1; in_hdr.pt = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid = 0xFACE0001u;
        const std::vector<uint8_t> ip_pkt(20, 0x45u);
        const auto frm = decodeGTPU(encodeGTPU(in_hdr, ip_pkt));
        ASSERT_TRUE(frm.has_value());

        const auto result = dp.processUplink(*frm);

        ASSERT_TRUE(result.forwarded);
        ASSERT_FALSE(result.dropped);
        ASSERT_FALSE(result.gated);
    }

    SECTION("DataPlane: UL gate CLOSED → packet gated (dropped)");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000056";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        PDRRule pdr{};
        pdr.pdr_id = 1; pdr.precedence = 100; pdr.far_id = 101; pdr.qer_id = 20;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(0xFACE0010u, 0x7f000001u);

        FARRule fr{}; fr.far_id = 101; fr.apply_action.forw = true;
        ForwardingParameters fp{};
        fp.destination_interface = InterfaceType::CORE;
        fp.outer_header_creation = FTEID::withIPv4(0xFACE0011u, 0x0a000001u);
        fr.forwarding_parameters = fp;

        // QER with UL gate CLOSED
        QERRule qer{};
        qer.qer_id  = 20;
        qer.ul_gate = GateStatus::CLOSED;
        qer.dl_gate = GateStatus::OPEN;

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_QER, qer.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            5600, 560, encodeIEs(outer)));

        auto session = h.getSession(5600);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        GTPUHeader in_hdr{}; in_hdr.version = 1; in_hdr.pt = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid = 0xFACE0010u;
        const std::vector<uint8_t> ip_pkt(20, 0x45u);
        const auto frm = decodeGTPU(encodeGTPU(in_hdr, ip_pkt));
        ASSERT_TRUE(frm.has_value());

        const auto result = dp.processUplink(*frm);

        ASSERT_FALSE(result.forwarded);
        ASSERT_TRUE(result.dropped);
        ASSERT_TRUE(result.gated);
        // UL octets still accounted
        ASSERT_EQ(session->getUplinkOctets(), uint64_t(20));
    }

    // =========================================================================
    // Group 9 — DataPlane: BUFF action
    // =========================================================================

    SECTION("DataPlane: BUFF action — packet stored in buffer");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000060";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        PDRRule pdr{};
        pdr.pdr_id = 1; pdr.precedence = 100; pdr.far_id = 101;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(0xBEEF0001u, 0x7f000001u);

        FARRule fr{}; fr.far_id = 101; fr.apply_action.buff = true;

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            5700, 570, encodeIEs(outer)));

        auto session = h.getSession(5700);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        const std::vector<uint8_t> ip_pkt = {0x45, 0x00, 0x00, 0x28, 0x00, 0x01, 0x00,
                                              0x00, 0x40, 0x11, 0x00, 0x00};
        GTPUHeader in_hdr{}; in_hdr.version = 1; in_hdr.pt = true;
        in_hdr.message_type = GTPUMessageType::G_PDU;
        in_hdr.teid = 0xBEEF0001u;
        const auto frm = decodeGTPU(encodeGTPU(in_hdr, ip_pkt));
        ASSERT_TRUE(frm.has_value());

        const auto result = dp.processUplink(*frm);

        ASSERT_FALSE(result.forwarded);
        ASSERT_FALSE(result.dropped);
        ASSERT_TRUE(result.buffered);
        ASSERT_EQ(session->getUplinkOctets(), static_cast<uint64_t>(ip_pkt.size()));
    }

    SECTION("DataPlane: drainBuffer returns buffered packets and clears buffer");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000061";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        PDRRule pdr{};
        pdr.pdr_id = 1; pdr.precedence = 100; pdr.far_id = 101;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid      = FTEID::withIPv4(0xBEEF0010u, 0x7f000001u);

        FARRule fr{}; fr.far_id = 101; fr.apply_action.buff = true;

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            5800, 580, encodeIEs(outer)));

        auto session = h.getSession(5800);
        ASSERT_TRUE(session != nullptr);

        DataPlane dp;
        dp.addSession(session);

        const std::vector<uint8_t> pkt1(10, 0xAAu);
        const std::vector<uint8_t> pkt2(15, 0xBBu);

        auto sendPkt = [&](const std::vector<uint8_t>& pkt) {
            GTPUHeader h2{}; h2.version = 1; h2.pt = true;
            h2.message_type = GTPUMessageType::G_PDU; h2.teid = 0xBEEF0010u;
            const auto frm = decodeGTPU(encodeGTPU(h2, pkt));
            if (frm) dp.processUplink(*frm);
        };

        sendPkt(pkt1);
        sendPkt(pkt2);

        // Buffer should have 2 packets
        const auto drained = dp.drainBuffer(5800);
        ASSERT_EQ(drained.size(), std::size_t(2));
        ASSERT_TRUE(drained[0] == pkt1);
        ASSERT_TRUE(drained[1] == pkt2);

        // Second drain returns empty (buffer cleared)
        const auto drained2 = dp.drainBuffer(5800);
        ASSERT_TRUE(drained2.empty());
    }

    // =========================================================================
    // Group 10 — SESSION_MODIFICATION: rule update via handleMessage
    // =========================================================================

    SECTION("SESSION_MODIFICATION: UPDATE_FAR changes apply_action in session");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000070";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        // Step 1: establish session with FORW FAR
        auto session = makeDPSession(h, 5900, 590, supi, 0x0CAF0001u, 0x0CAF0002u);
        ASSERT_TRUE(session != nullptr);
        ASSERT_EQ(session->getFARs().size(), std::size_t(1));
        ASSERT_TRUE(session->getFARs()[0].apply_action.forw);

        // Step 2: modify session — send UPDATE_FAR to change action to DROP
        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        FARRule updated_far{};
        updated_far.far_id = 101;   // same far_id as installed
        updated_far.apply_action.drop = true;
        updated_far.apply_action.forw = false;

        std::vector<InformationElement> mod_outer;
        mod_outer.push_back(wrap(IEType::UPDATE_FAR, updated_far.toIEs()));

        const bool ok = h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_MODIFICATION_REQUEST,
            5900, 591, encodeIEs(mod_outer)));

        ASSERT_TRUE(ok);
        ASSERT_EQ(session->getFARs().size(), std::size_t(1));
        ASSERT_TRUE(session->getFARs()[0].apply_action.drop);
        ASSERT_FALSE(session->getFARs()[0].apply_action.forw);
    }

    SECTION("SESSION_MODIFICATION: CREATE_QER adds new QER to session");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000071";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto session = makeDPSession(h, 6000, 600, supi, 0x0CA00001u, 0x0CA00002u);
        ASSERT_TRUE(session != nullptr);
        ASSERT_EQ(session->getQERs().size(), std::size_t(0));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        QERRule new_qer{};
        new_qer.qer_id  = 99;
        new_qer.ul_gate = GateStatus::OPEN;
        new_qer.dl_gate = GateStatus::OPEN;
        new_qer.ul_mbr  = 50000u;

        std::vector<InformationElement> mod_outer;
        mod_outer.push_back(wrap(IEType::CREATE_QER, new_qer.toIEs()));

        const bool ok = h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_MODIFICATION_REQUEST,
            6000, 601, encodeIEs(mod_outer)));

        ASSERT_TRUE(ok);
        ASSERT_EQ(session->getQERs().size(), std::size_t(1));
        ASSERT_EQ(session->getQERs()[0].qer_id, uint32_t(99));
        ASSERT_EQ(session->getQERs()[0].ul_mbr,  uint64_t(50000));
    }

    SECTION("PFCPSession::updateRules: UPDATE_QER replaces existing + UPDATE_FAR adds new");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-250010000000072";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        PDRRule pdr{}; pdr.pdr_id = 1; pdr.precedence = 100;
        pdr.far_id = 1; pdr.qer_id = 5;
        pdr.pdi.source_interface = InterfaceType::ACCESS;
        pdr.pdi.local_fteid = FTEID::withIPv4(0x0CAFE001u, 0x7f000001u);

        FARRule fr{}; fr.far_id = 1; fr.apply_action.forw = true;
        ForwardingParameters fp{}; fp.destination_interface = InterfaceType::CORE;
        fp.outer_header_creation = FTEID::withIPv4(0x0CAFE002u, 0x0a000001u);
        fr.forwarding_parameters = fp;

        QERRule orig_qer{}; orig_qer.qer_id = 5;
        orig_qer.ul_gate = GateStatus::OPEN; orig_qer.dl_gate = GateStatus::OPEN;

        std::vector<InformationElement> est_outer;
        est_outer.push_back(InformationElement::fromSupi(supi));
        est_outer.push_back(wrap(IEType::CREATE_PDR, pdr.toIEs()));
        est_outer.push_back(wrap(IEType::CREATE_FAR, fr.toIEs()));
        est_outer.push_back(wrap(IEType::CREATE_QER, orig_qer.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            6100, 610, encodeIEs(est_outer)));

        auto session = h.getSession(6100);
        ASSERT_TRUE(session != nullptr);
        ASSERT_EQ(session->getQERs().size(), std::size_t(1));
        ASSERT_TRUE(session->getQERs()[0].ul_gate == GateStatus::OPEN);

        // Modify: update QER 5 to close UL gate + add a new FAR 2
        QERRule upd_qer{}; upd_qer.qer_id = 5;
        upd_qer.ul_gate = GateStatus::CLOSED; upd_qer.dl_gate = GateStatus::OPEN;

        FARRule new_far{}; new_far.far_id = 2; new_far.apply_action.drop = true;

        std::vector<InformationElement> mod_outer;
        mod_outer.push_back(wrap(IEType::UPDATE_QER, upd_qer.toIEs()));
        mod_outer.push_back(wrap(IEType::UPDATE_FAR, new_far.toIEs()));

        const bool ok = h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_MODIFICATION_REQUEST,
            6100, 611, encodeIEs(mod_outer)));

        ASSERT_TRUE(ok);
        // QER 5 replaced
        ASSERT_EQ(session->getQERs().size(), std::size_t(1));
        ASSERT_TRUE(session->getQERs()[0].ul_gate == GateStatus::CLOSED);
        // FAR 2 appended (FAR 1 still present)
        ASSERT_EQ(session->getFARs().size(), std::size_t(2));
        const QERRule* q = session->findQER(5);
        ASSERT_TRUE(q != nullptr);
        ASSERT_TRUE(q->ul_gate == GateStatus::CLOSED);
    }

    // -----------------------------------------------------------------------
    // Group 11 — GTP-U PDU Session Container extension header (TS 38.415)
    // -----------------------------------------------------------------------

    SECTION("GTP-U PSC ext hdr: UL, QFI=5 encode/decode round-trip");
    {
        GTPUHeader hdr{}; hdr.version = 1; hdr.pt = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid = 0xABCD0056u;

        const PduSessionContainer psc_in{ true, 5u }; // uplink=true, qfi=5
        const std::vector<uint8_t> payload(12, 0x60u); // dummy IPv6 header start

        const auto wire = encodeGTPU(hdr, payload, psc_in);

        // E flag forces 12-byte header; then 4-byte PSC ext hdr; then payload
        ASSERT_TRUE(wire.size() == 12u + 4u + 12u);

        // Decode and verify
        const auto frm_opt = decodeGTPU(wire);
        ASSERT_TRUE(frm_opt.has_value());
        ASSERT_TRUE(frm_opt->header.e);
        ASSERT_EQ(frm_opt->header.teid, 0xABCD0056u);
        ASSERT_EQ(frm_opt->header.next_ext_hdr, GTPU_EXT_HDR_PDU_SESSION_CONTAINER);
        ASSERT_TRUE(frm_opt->pdu_session_container.has_value());
        ASSERT_TRUE(frm_opt->pdu_session_container->uplink == true);
        ASSERT_EQ(frm_opt->pdu_session_container->qfi, uint8_t(5));
        ASSERT_TRUE(frm_opt->payload == payload);
    }

    SECTION("GTP-U PSC ext hdr: DL, QFI=9 encode/decode round-trip");
    {
        GTPUHeader hdr{}; hdr.version = 1; hdr.pt = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid = 0xABCD0057u;

        const PduSessionContainer psc_in{ false, 9u }; // uplink=false (DL), qfi=9
        const std::vector<uint8_t> payload = {0x45, 0x00, 0x00, 0x14};

        const auto wire = encodeGTPU(hdr, payload, psc_in);
        ASSERT_TRUE(wire.size() == 12u + 4u + 4u);

        const auto frm_opt = decodeGTPU(wire);
        ASSERT_TRUE(frm_opt.has_value());
        ASSERT_TRUE(frm_opt->header.e);
        ASSERT_EQ(frm_opt->header.next_ext_hdr, GTPU_EXT_HDR_PDU_SESSION_CONTAINER);
        ASSERT_TRUE(frm_opt->pdu_session_container.has_value());
        ASSERT_TRUE(frm_opt->pdu_session_container->uplink == false);
        ASSERT_EQ(frm_opt->pdu_session_container->qfi, uint8_t(9));
        ASSERT_TRUE(frm_opt->payload == payload);
    }

    SECTION("GTP-U without PSC: pdu_session_container is empty after decode");
    {
        GTPUHeader hdr{}; hdr.version = 1; hdr.pt = true;
        hdr.message_type = GTPUMessageType::G_PDU;
        hdr.teid = 0xABCD0058u;
        const std::vector<uint8_t> payload(8, 0x00u);
        const auto frm_opt = decodeGTPU(encodeGTPU(hdr, payload));
        ASSERT_TRUE(frm_opt.has_value());
        ASSERT_FALSE(frm_opt->pdu_session_container.has_value());
        ASSERT_TRUE(frm_opt->payload == payload);
    }

    // -----------------------------------------------------------------------
    // Group 12 — Periodic usage reporting timer (TS 29.244 §8.2.41 PERIO)
    // -----------------------------------------------------------------------

    SECTION("tickPeriodicReports: init tick doesn't fire; fires after measurement_period");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-206010000000058";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        URRRule urr{};
        urr.urr_id                   = 201u;
        urr.measurement_method.volume = true;
        urr.reporting_triggers.perio  = true;
        urr.measurement_period        = 2u; // 2 seconds

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_URR, urr.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            7000, 700, encodeIEs(outer)));

        auto session = h.getSession(7000);
        ASSERT_TRUE(session != nullptr);

        const auto t0 = std::chrono::steady_clock::now();

        // First tick initialises timer — no report
        const auto r0 = session->tickPeriodicReports(t0);
        ASSERT_EQ(r0.size(), std::size_t(0));

        // t0 + 1s: period=2s not yet elapsed
        const auto r1 = session->tickPeriodicReports(t0 + std::chrono::seconds(1));
        ASSERT_EQ(r1.size(), std::size_t(0));

        // t0 + 3s: 3s >= 2s → fires once
        const auto r2 = session->tickPeriodicReports(t0 + std::chrono::seconds(3));
        ASSERT_EQ(r2.size(), std::size_t(1));
        ASSERT_EQ(r2[0].urr_id,  uint32_t(201));
        ASSERT_EQ(r2[0].ur_seqn, uint32_t(1));
        ASSERT_TRUE(r2[0].trigger.perio);
    }

    SECTION("tickPeriodicReports: second fire at t0+5s, ur_seqn increments to 2");
    {
        PFCPHandler h(8805);
        setupHandler(h);
        const std::string supi = "imsi-206010000000059";
        h.notifyAuthenticationResult(makeAuthResult(supi));

        auto wrap = [](IEType t, const std::vector<InformationElement>& inner) {
            InformationElement g{}; g.type = t; g.value = encodeIEs(inner); return g;
        };

        URRRule urr{};
        urr.urr_id                   = 202u;
        urr.measurement_method.volume = true;
        urr.reporting_triggers.perio  = true;
        urr.measurement_period        = 2u;

        std::vector<InformationElement> outer;
        outer.push_back(InformationElement::fromSupi(supi));
        outer.push_back(wrap(IEType::CREATE_URR, urr.toIEs()));

        h.handleMessage(makeSessionPDU(
            PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST,
            7100, 710, encodeIEs(outer)));

        auto session = h.getSession(7100);
        ASSERT_TRUE(session != nullptr);

        const auto t0 = std::chrono::steady_clock::now();
        session->tickPeriodicReports(t0);                             // init → last = t0
        session->tickPeriodicReports(t0 + std::chrono::seconds(3));   // 1st fire → last = t0+2s
        // At t0+5s: now - last = 3s >= 2s → 2nd fire
        const auto r = session->tickPeriodicReports(t0 + std::chrono::seconds(5));
        ASSERT_EQ(r.size(), std::size_t(1));
        ASSERT_EQ(r[0].urr_id,  uint32_t(202));
        ASSERT_EQ(r[0].ur_seqn, uint32_t(2));
        ASSERT_TRUE(r[0].trigger.perio);
    }

    // -----------------------------------------------------------------------
    // Group 13 — MTU / IP fragmentation (TS 29.244 §7.1)
    // -----------------------------------------------------------------------

    SECTION("MTU constants: PFCP_ETH_MTU=1472, PFCP_UDP_HARD_LIMIT=65507");
    {
        ASSERT_EQ(PFCP_ETH_MTU,        std::size_t(1472));
        ASSERT_EQ(PFCP_UDP_HARD_LIMIT,  std::size_t(65507));
    }

    SECTION("PFCPSocket: default mtu() == PFCP_ETH_MTU; setMtu() updates it");
    {
        PFCPSocket sock(18805);  // arbitrary port — socket not opened
        ASSERT_EQ(sock.mtu(), PFCP_ETH_MTU);
        sock.setMtu(576u);   // RFC 791 minimum reassembly size
        ASSERT_EQ(sock.mtu(), std::size_t(576));
        sock.setMtu(PFCP_ETH_MTU);  // restore
        ASSERT_EQ(sock.mtu(), PFCP_ETH_MTU);
    }

    // -----------------------------------------------------------------------
    std::cout << "\n=== Results: " << g_pass << " passed, " << g_fail << " failed ===\n";
    return (g_fail == 0) ? 0 : 1;
}
