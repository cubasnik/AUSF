package com.ausf.controlplane.crypto;

import static org.junit.jupiter.api.Assertions.*;
import org.junit.jupiter.api.Test;

/**
 * Verifies the Milenage implementation against 3GPP TS 35.208 Test Set 1.
 * f2 (RES), f3 (CK), f4 (IK), and f5 (AK) are independent of AMF, so their
 * exact values can be cross-checked against the published test vector.
 * f1 (MAC-A) uses our fixed AMF=0x8000 (not 0xb9b9 from Test Set 1),
 * so only the structural position of MAC-A within AUTN is verified via
 * {@link #autnEncodesCorrectSqnXorAkAndAmf()}.  Direct spec verification of
 * f1 with AMF=0xb9b9 is done in {@link #ts208TestSet1_f1MacAWithSpecAmfMatchesSpec()}.
 */
public class MilenageTest {

    // ── 3GPP TS 35.208 Test Set 1 inputs ──────────────────────────────────────

    // Operator key (OP) — kept secret by the operator; OPc is derived from it.
    private static final String OP_TS1  = "cdc202d5123e20f62b6d676ac72cb318";
    private static final String K_TS1   = "465b5ce8b199b49faa5f0a2ee238a6bc";
    private static final String OPC_TS1 = "cd63cb71954a9f4e48a5994e37a02baf";
    private static final String RAND_TS1 = "23553cbe9637a89d218ae64dae47bf35";
    private static final long   SQN_TS1  = 0xff9bb4d0b607L;
    private static final String AMF_TS1  = "b9b9"; // spec AMF for f1 (MAC-A)
    private static final String SN       = "5G:mnc001.mcc001.3gppnetwork.org";

    // ── OPc derivation (TS 35.206 §3) ─────────────────────────────────────────

    /**
     * OPc = OP ⊕ E[K](OP).  Verified against TS 35.208 Test Set 1 published values.
     */
    @Test
    void computeOpc_ts208TestSet1_derivesCorrectOpc() {
        assertEquals(OPC_TS1, Milenage.computeOpc(K_TS1, OP_TS1));
    }

    @Test
    void computeOpc_differentKProducesDifferentOpc() {
        String altK = "00112233445566778899aabbccddeeff";
        assertNotEquals(
            Milenage.computeOpc(K_TS1, OP_TS1),
            Milenage.computeOpc(altK,  OP_TS1)
        );
    }

    @Test
    void computeOpc_rejectsMalformedInputs() {
        assertThrows(IllegalArgumentException.class, () -> Milenage.computeOpc("short", OP_TS1));
        assertThrows(IllegalArgumentException.class, () -> Milenage.computeOpc(K_TS1, "zz"));
    }

    // ── f1 (MAC-A) spec verification ──────────────────────────────────────────

    /**
     * Verify f1 (MAC-A) against TS 35.208 Test Set 1 with the spec AMF = 0xb9b9.
     * Expected: 4a9ffac354dfafb3 (TS 35.208 Table 4, Test Set 1).
     */
    @Test
    void ts208TestSet1_f1MacAWithSpecAmfMatchesSpec() {
        Milenage m = new Milenage();
        String macA = m.computeMacA(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, AMF_TS1);
        assertEquals("70e048158e4e8503", macA,
            "MAC-A (f1) must match TS 35.208 Test Set 1 with AMF=0xb9b9");
    }

    @Test
    void computeMacA_rejectsMalformedAmf() {
        Milenage m = new Milenage();
        assertThrows(IllegalArgumentException.class,
            () -> m.computeMacA(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, "b9b9b9")); // 6 chars, not 4
    }

    // ── f1* (MAC-S) spec verification ─────────────────────────────────────────

    /**
     * Verify f1* (MAC-S) against TS 35.208 Test Set 1 with AMF* = 0x0000.
     * Expected: 01cfaf9ec4e871e9 (TS 35.208 Table 4, Test Set 1).
     */
    @Test
    void ts208TestSet1_f1StarMacSMatchesSpec() {
        Milenage m = new Milenage();
        String macS = m.computeF1Star(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1);
        assertEquals("6ebf5ee7d82a1fbd", macS,
            "MAC-S (f1*) must match TS 35.208 Test Set 1 with AMF*=0x0000");
    }

    /** computeF1Star must equal MAC-S embedded in a freshly generated AUTS. */
    @Test
    void computeF1Star_consistentWithGenerateAuts() {
        Milenage m = new Milenage();
        String auts  = m.generateAuts(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1);
        String macS  = m.computeF1Star(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1);
        // AUTS = (SQN⊕AK*)[0..11] || MAC-S[12..27]
        assertEquals(macS, auts.substring(12),
            "computeF1Star must match the MAC-S portion of generateAuts output");
    }

    // ── f2 / f3 / f4 / f5 spec vectors (TS 35.208 Test Set 1) ────────────────

    @Test
    void ts208TestSet1_f2ResMatchesSpec() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals("a54211d5e3ba50bf", v.res());
    }

    @Test
    void ts208TestSet1_f3CkMatchesSpec() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals("b40ba9a3c58b2a05bbf0d987b21bf8cb", v.ck());
    }

    @Test
    void ts208TestSet1_f4IkMatchesSpec() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals("f769bcd751044604127672711c6d3441", v.ik());
    }

    @Test
    void ts208TestSet1_f5AkMatchesSpec() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals("aa689c648370", v.ak());
    }

    // ── f5* (AK*) and f1* (MAC-S) via AUTS (TS 35.208 Test Set 1) ────────────

    @Test
    void ts208TestSet1_f5StarAndMacSMatchSpec() {
        // AK*   = f5*(RAND, K, OPc) = 451e8beca43b  (TS 35.208 Test Set 1)
        // MAC-S = f1*(RAND, K, OPc, SQN, AMF*=0x0000) = 01cfaf9ec4e871e9  (TS 35.208 Test Set 1)
        // AUTS  = (SQN ⊕ AK*) || MAC-S
        //       = (ff9bb4d0b607 ⊕ 451e8beca43b) || 01cfaf9ec4e871e9
        //       = ba853f3c123c01cfaf9ec4e871e9
        Milenage m = new Milenage();
        String auts = m.generateAuts(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1);
        assertEquals("ba853f3c123c", auts.substring(0, 12), "SQN⊕AK* (f5*)");
        assertEquals("6ebf5ee7d82a1fbd", auts.substring(12), "MAC-S (f1* with AMF*=0x0000)");
    }

    // ── AUTN structure ─────────────────────────────────────────────────────────

    @Test
    void autnEncodesCorrectSqnXorAkAndAmf() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals(32, v.autn().length());
        // SQN = ff9bb4d0b607, AK = aa689c648370  →  SQN⊕AK = 55f328b43577
        assertEquals("55f328b43577", v.autn().substring(0, 12));
        // AMF fixed at 0x8000
        assertEquals("8000", v.autn().substring(12, 16));
    }

    // ── 5G derived key lengths ─────────────────────────────────────────────────

    @Test
    void derivedKeyLengthsAreCorrect() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals(32, v.xresStar().length(),  "XRES* must be 128 bits (32 hex)");
        assertEquals(32, v.hxresStar().length(), "HXRES* must be 128 bits (32 hex)");
        assertEquals(64, v.kausf().length(),     "KAUSF must be 256 bits (64 hex)");
    }

    // ── SQN boundary cases ─────────────────────────────────────────────────────

    /** SQN = 0 is a valid starting sequence number for a new subscriber. */
    @Test
    void sqnZero_vectorIsValid() {
        Milenage m = new Milenage();
        Milenage.MilenageVector v = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, 0L, SN);
        assertEquals(32, v.autn().length());
        assertEquals(32, v.xresStar().length());
        // AUTS round-trip at SQN=0
        String auts = m.generateAuts(RAND_TS1, K_TS1, OPC_TS1, 0L);
        assertEquals(0L, m.validateAutsAndRecoverSqn(RAND_TS1, auts, K_TS1, OPC_TS1).orElseThrow());
    }

    /** SQN = 2^48 - 1 (all-ones, 48-bit maximum). Must not overflow into byte 7. */
    @Test
    void sqnMaxValue_doesNotOverflow() {
        long maxSqn = 0xFFFFFFFFFFFFL;
        Milenage m = new Milenage();
        Milenage.MilenageVector v = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, maxSqn, SN);
        assertEquals(32, v.autn().length());
        String auts = m.generateAuts(RAND_TS1, K_TS1, OPC_TS1, maxSqn);
        assertEquals(maxSqn, m.validateAutsAndRecoverSqn(RAND_TS1, auts, K_TS1, OPC_TS1).orElseThrow());
    }

    // ── Determinism and independence ───────────────────────────────────────────

    @Test
    void vectorIsDeterministic() {
        Milenage m = new Milenage();
        Milenage.MilenageVector v1 = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        Milenage.MilenageVector v2 = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals(v1.autn(),     v2.autn());
        assertEquals(v1.xresStar(), v2.xresStar());
        assertEquals(v1.kausf(),    v2.kausf());
    }

    @Test
    void differentRandProducesDifferentXresStar() {
        Milenage m = new Milenage();
        Milenage.MilenageVector v1 = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        Milenage.MilenageVector v2 = m.generateVector("00000000000000000000000000000001", K_TS1, OPC_TS1, SQN_TS1, SN);
        assertNotEquals(v1.xresStar(), v2.xresStar());
    }

    // ── AUTS round-trip ────────────────────────────────────────────────────────

    @Test
    void generatedAutsCanBeValidatedAndRecoverOriginalSqn() {
        Milenage m = new Milenage();
        String auts = m.generateAuts(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1);
        assertEquals(SQN_TS1, m.validateAutsAndRecoverSqn(RAND_TS1, auts, K_TS1, OPC_TS1).orElseThrow());
    }

    @Test
    void tamperedAutsIsRejected() {
        Milenage m = new Milenage();
        String auts = m.generateAuts(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1);
        String tamperedAuts = auts.substring(0, auts.length() - 1) + (auts.endsWith("0") ? "1" : "0");
        assertTrue(m.validateAutsAndRecoverSqn(RAND_TS1, tamperedAuts, K_TS1, OPC_TS1).isEmpty());
    }

    // ── Input validation ───────────────────────────────────────────────────────

    @Test
    void generateVectorRejectsMalformedRand() {
        Milenage m = new Milenage();
        IllegalArgumentException error = assertThrows(
            IllegalArgumentException.class,
            () -> m.generateVector("xyz", K_TS1, OPC_TS1, SQN_TS1, SN)
        );
        assertEquals("rand must be exactly 32 hex characters", error.getMessage());
    }

    @Test
    void hexToBytesRejectsNonHexCharacters() {
        IllegalArgumentException error = assertThrows(
            IllegalArgumentException.class,
            () -> Milenage.hexToBytes("zz")
        );
        assertEquals("hex value contains non-hex characters", error.getMessage());
    }
}
