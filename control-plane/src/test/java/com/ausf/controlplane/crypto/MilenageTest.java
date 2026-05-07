package com.ausf.controlplane.crypto;

import static org.junit.jupiter.api.Assertions.*;
import org.junit.jupiter.api.Test;

/**
 * Verifies the Milenage implementation against 3GPP TS 35.208 Test Set 1.
 * f2 (RES), f3 (CK), f4 (IK), and f5 (AK) are independent of AMF, so their
 * exact values can be cross-checked against the published test vector.
 * f1 (MAC-A) uses our fixed AMF=0x8000 (not 0xb9b9 from Test Set 1),
 * so only the structural position of MAC-A within AUTN is verified.
 */
public class MilenageTest {

    // 3GPP TS 35.208 Test Set 1 inputs
    private static final String K_TS1    = "465b5ce8b199b49faa5f0a2ee238a6bc";
    private static final String OPC_TS1  = "cd63cb71954a9f4e48a5994e37a02baf";
    private static final String RAND_TS1 = "23553cbe9637a89d218ae64dae47bf35";
    private static final long   SQN_TS1  = 0xff9bb4d0b607L;
    private static final String SN       = "5G:mnc001.mcc001.3gppnetwork.org";

    @Test
    void ts208TestSet1_f2ResMatchesSpec() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        // Expected: 3GPP TS 35.208 Test Set 1 — f2 is AMF-independent
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
        // Expected: 3GPP TS 35.208 Test Set 1 — f5 is AMF-independent
        assertEquals("aa689c648370", v.ak());
    }

    @Test
    void autnEncodesCorrectSqnXorAkAndAmf() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        // AUTN = (SQN⊕AK) || AMF || MAC-A  → 16 bytes = 32 hex chars
        assertEquals(32, v.autn().length());
        // SQN = ff9bb4d0b607, AK = aa689c648370  →  SQN⊕AK = 55f328b43577
        assertEquals("55f328b43577", v.autn().substring(0, 12));
        // AMF fixed at 0x8000
        assertEquals("8000", v.autn().substring(12, 16));
    }

    @Test
    void derivedKeyLengthsAreCorrect() {
        Milenage.MilenageVector v = new Milenage().generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals(32, v.xresStar().length(),  "XRES* must be 128 bits (32 hex)");
        assertEquals(32, v.hxresStar().length(), "HXRES* must be 128 bits (32 hex)");
        assertEquals(64, v.kausf().length(),     "KAUSF must be 256 bits (64 hex)");
    }

    @Test
    void vectorIsDeterministic() {
        Milenage m = new Milenage();
        Milenage.MilenageVector v1 = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        Milenage.MilenageVector v2 = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        assertEquals(v1.autn(),      v2.autn());
        assertEquals(v1.xresStar(),  v2.xresStar());
        assertEquals(v1.kausf(),     v2.kausf());
    }

    @Test
    void differentRandProducesDifferentXresStar() {
        Milenage m = new Milenage();
        Milenage.MilenageVector v1 = m.generateVector(RAND_TS1, K_TS1, OPC_TS1, SQN_TS1, SN);
        Milenage.MilenageVector v2 = m.generateVector("00000000000000000000000000000001", K_TS1, OPC_TS1, SQN_TS1, SN);
        assertNotEquals(v1.xresStar(), v2.xresStar());
    }

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