package com.ausf.controlplane.crypto;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import org.junit.jupiter.api.Test;

/**
 * Verifies the TUAK implementation against public 3GPP-oriented reference vectors.
 */
public class TuakTest {

    private static final String K_61 = "abababababababababababababababab";
    private static final String RAND_61 = "42424242424242424242424242424242";
    private static final long SQN_61 = 0x111111111111L;
    private static final String AMF_61 = "ffff";
    private static final String TOP_61 = "5555555555555555555555555555555555555555555555555555555555555555";
    private static final String TOPC_61 = "bd04d9530e87513c5d837ac2ad954623a8e2330c115305a73eb45d1f40cccbff";

    private static final String K_72 = "fffefdfcfbfaf9f8f7f6f5f4f3f2f1f0efeeedecebeae9e8e7e6e5e4e3e2e1e0";
    private static final String RAND_72 = "0123456789abcdef0123456789abcdef";
    private static final String TOP_72 = "808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f";

    private static final String K = K_61;
    private static final String TOPC = TOPC_61;
    private static final String RAND = RAND_61;
    private static final long SQN = SQN_61;
    private static final String SN = "5G:mnc001.mcc001.3gppnetwork.org";

    @Test
    void deriveTopcMatchesReferenceVector() {
        Tuak tuak = new Tuak();

        assertEquals(TOPC_61, tuak.deriveTopc(K_61, TOP_61));
    }

    @Test
    void macFunctionsMatchReferenceVector61() {
        Tuak tuak = new Tuak();

        assertEquals("f9a54e6aeaa8618d", tuak.generateMacA(RAND_61, K_61, TOPC_61, SQN_61, AMF_61));
        assertEquals("e94b4dc6c7297df3", tuak.generateMacS(RAND_61, K_61, TOPC_61, SQN_61, AMF_61));
    }

    @Test
    void f2345AndF5StarMatchReferenceVector72() {
        Tuak tuak = new Tuak();
        String topc72 = tuak.deriveTopc(K_72, TOP_72);

        Tuak.TuakF2345Vector vector = tuak.generateF2345(RAND_72, K_72, topc72);

        assertEquals("e9d749dc4eea0035", vector.res());
        assertEquals("a4cb6f6529ab17f8337f27baa8234d47", vector.ck());
        assertEquals("2274155ccf4199d5e2abcbf621907f90", vector.ik());
        assertEquals("480a9345cc1e", vector.ak());
        assertEquals("f84eb338848c", tuak.generateAkStar(RAND_72, K_72, topc72));
    }

    @Test
    void vectorIsDeterministic() {
        Tuak t = new Tuak();
        Tuak.TuakVector v1 = t.generateVector(RAND, K, TOPC, SQN, SN);
        Tuak.TuakVector v2 = t.generateVector(RAND, K, TOPC, SQN, SN);

        assertEquals(v1.autn(), v2.autn());
        assertEquals(v1.xresStar(), v2.xresStar());
        assertEquals(v1.kausf(), v2.kausf());
    }

    @Test
    void differentRandProducesDifferentXresStar() {
        Tuak t = new Tuak();
        Tuak.TuakVector v1 = t.generateVector(RAND, K, TOPC, SQN, SN);
        Tuak.TuakVector v2 = t.generateVector("00000000000000000000000000000001", K, TOPC, SQN, SN);

        assertNotEquals(v1.xresStar(), v2.xresStar());
    }

    @Test
    void tuakAndMilenageProduceDifferentXresStar() {
        Milenage.MilenageVector mv = new Milenage().generateVector(RAND, K, "e8ed289deba952e4283b54e88e6183ca", SQN, SN);
        Tuak.TuakVector tv = new Tuak().generateVector(RAND, K, TOPC, SQN, SN);

        assertNotEquals(mv.xresStar(), tv.xresStar(),
            "Milenage (AES) and TUAK (Keccak) must produce distinct XRES* values");
    }

    @Test
    void generateVectorRejectsNonTopcInput() {
        Tuak tuak = new Tuak();

        IllegalArgumentException error = assertThrows(
            IllegalArgumentException.class,
            () -> tuak.generateVector(RAND, K, "abcd", SQN, SN)
        );

        assertEquals("topc must be exactly 64 hex characters", error.getMessage());
    }
}