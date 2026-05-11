package com.ausf.controlplane.crypto;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.OptionalLong;
import javax.crypto.Cipher;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * Milenage algorithm per 3GPP TS 35.206, using AES-128 as the block cipher E.
 * 5G key derivations (XRES*, HXRES*, KAUSF) follow 3GPP TS 33.501 Annex A.
 */
public class Milenage {

    // Byte-level left-rotation constants (r1–r5 from TS 35.206 Table 1)
    private static final int R1 = 8, R2 = 0, R3 = 4, R4 = 8, R5 = 12;
    // Differentiation constants XOR-ed into the last byte (c1–c5)
    private static final int C1 = 0x00, C2 = 0x01, C3 = 0x02, C4 = 0x04, C5 = 0x08;
    // AMF = 0x8000 (normal authentication, TS 33.102)
    private static final byte[] DEFAULT_AMF = {(byte) 0x80, (byte) 0x00};
    private static final byte[] RESYNC_AMF = {(byte) 0x00, (byte) 0x00};

    /**
     * Full 5G Milenage vector.
     *
     * @param rand      RAND  – 32 hex chars (16 bytes)
     * @param autn      AUTN  – 32 hex chars: (SQN⊕AK)||AMF||MAC-A
     * @param res       RES   – 16 hex chars: f2 output
     * @param ck        CK    – 32 hex chars: f3 output
     * @param ik        IK    – 32 hex chars: f4 output
     * @param ak        AK    – 12 hex chars: f5 output (anonymity key)
     * @param xresStar  XRES* – 32 hex chars: 5G derived per TS 33.501 A.4
     * @param hxresStar HXRES*– 32 hex chars: SHA-256(RAND||XRES*)[0:16]
     * @param kausf     KAUSF – 64 hex chars: 5G derived per TS 33.501 A.2
     */
    public record MilenageVector(
        String rand,
        String autn,
        String res,
        String ck,
        String ik,
        String ak,
        String xresStar,
        String hxresStar,
        String kausf
    ) {}

    /**
     * Compute the full 5G AKA Milenage authentication vector.
     *
     * @param rand              16-byte RAND as 32 hex chars
     * @param permanentKey      K: 16-byte AES key as 32 hex chars
     * @param opc               OPc: 16-byte operator constant as 32 hex chars
     * @param sqn               48-bit sequence number
     * @param servingNetworkName e.g. "5G:mnc001.mcc001.3gppnetwork.org"
     */
    public MilenageVector generateVector(
        String rand,
        String permanentKey,
        String opc,
        long sqn,
        String servingNetworkName
    ) {
        requireHexLength(rand, 32, "rand");
        requireHexLength(permanentKey, 32, "permanentKey");
        requireHexLength(opc, 32, "opc");

        byte[] K    = hexToBytes(permanentKey);
        byte[] RAND = hexToBytes(rand);
        byte[] OPc  = hexToBytes(opc);
        byte[] SQN  = sqnToBytes(sqn);

        // TEMP = E[K](RAND ⊕ OPc)
        byte[] TEMP = aesEncrypt(K, xor(RAND, OPc));

        // f2 / f5: OUT2 = E[K](rotate(TEMP⊕OPc, R2) ⊕ c2) ⊕ OPc
        byte[] OUT2 = xor(aesEncrypt(K, xorLastByte(rotate(xor(TEMP, OPc), R2), C2)), OPc);
        byte[] RES  = subarray(OUT2, 8, 8);   // bits 64–127
        byte[] AK   = subarray(OUT2, 0, 6);   // bits 0–47

        // f1: MAC-A = OUT1[0..7] where OUT1 = E[K](IN1 ⊕ rotate(TEMP⊕OPc,R1) ⊕ c1) ⊕ OPc
        byte[] IN1   = buildIN1(SQN, DEFAULT_AMF);
        byte[] OUT1  = xor(aesEncrypt(K, xorLastByte(xor(IN1, rotate(xor(TEMP, OPc), R1)), C1)), OPc);
        byte[] MAC_A = subarray(OUT1, 0, 8);

        // AUTN = (SQN ⊕ AK) || AMF || MAC-A  → 16 bytes
        byte[] sqnXorAk = xor(SQN, AK);
        byte[] AUTN = concat(concat(sqnXorAk, DEFAULT_AMF), MAC_A);

        // f3: CK
        byte[] CK = xor(aesEncrypt(K, xorLastByte(rotate(xor(TEMP, OPc), R3), C3)), OPc);
        // f4: IK
        byte[] IK = xor(aesEncrypt(K, xorLastByte(rotate(xor(TEMP, OPc), R4), C4)), OPc);

        // 5G derivations
        byte[] ckIk      = concat(CK, IK);
        byte[] xresStar  = deriveXresStar(ckIk, servingNetworkName, RAND, RES);
        byte[] hxresStar = sha256Prefix(concat(RAND, xresStar), 16);
        byte[] kausf     = deriveKausf(ckIk, servingNetworkName, sqnXorAk);

        return new MilenageVector(
            rand,
            bytesToHex(AUTN),
            bytesToHex(RES),
            bytesToHex(CK),
            bytesToHex(IK),
            bytesToHex(AK),
            bytesToHex(xresStar),
            bytesToHex(hxresStar),
            bytesToHex(kausf)
        );
    }

    public String generateAuts(String rand, String permanentKey, String opc, long sqn) {
        requireHexLength(rand, 32, "rand");
        requireHexLength(permanentKey, 32, "permanentKey");
        requireHexLength(opc, 32, "opc");

        byte[] K = hexToBytes(permanentKey);
        byte[] RAND = hexToBytes(rand);
        byte[] OPc = hexToBytes(opc);
        byte[] SQN = sqnToBytes(sqn);
        byte[] TEMP = computeTemp(K, RAND, OPc);
        byte[] akStar = computeAkStar(K, TEMP, OPc);
        byte[] concealedSqn = xor(SQN, akStar);
        byte[] macS = computeMacS(K, TEMP, OPc, SQN);
        return bytesToHex(concat(concealedSqn, macS));
    }

    /**
     * Derive OPc from operator key OP and subscriber key K per TS 35.206 §3.
     * <pre>OPc = OP ⊕ E[K](OP)</pre>
     * In production deployments the operator holds OP as a secret and distributes
     * OPc (= OP ⊕ E[K](OP)) per-subscriber so that OP is never exposed to the HLR/UDM.
     *
     * @param k  16-byte AES key as 32 hex chars
     * @param op 16-byte operator constant as 32 hex chars
     * @return OPc as 32 hex chars
     */
    public static String computeOpc(String k, String op) {
        requireHexLength(k, 32, "k");
        requireHexLength(op, 32, "op");
        byte[] K  = hexToBytes(k);
        byte[] OP = hexToBytes(op);
        return bytesToHex(xor(OP, aesEncrypt(K, OP)));
    }

    /**
     * Compute MAC-A (f1) with a caller-supplied AMF value.
     * <p>The default {@link #generateVector} always uses AMF=0x8000. This method
     * exposes the raw f1 function so that the TS 35.208 spec vectors (which use
     * AMF=0xb9b9) can be verified directly.
     *
     * @param rand   32 hex chars (16 bytes)
     * @param k      32 hex chars (16 bytes)
     * @param opc    32 hex chars (16 bytes)
     * @param sqn    48-bit sequence number
     * @param amfHex 4 hex chars (2 bytes)
     * @return MAC-A as 16 hex chars (8 bytes)
     */
    public String computeMacA(String rand, String k, String opc, long sqn, String amfHex) {
        requireHexLength(rand,   32, "rand");
        requireHexLength(k,      32, "k");
        requireHexLength(opc,    32, "opc");
        requireHexLength(amfHex,  4, "amfHex");
        byte[] K    = hexToBytes(k);
        byte[] RAND = hexToBytes(rand);
        byte[] OPc  = hexToBytes(opc);
        byte[] SQN  = sqnToBytes(sqn);
        byte[] AMF  = hexToBytes(amfHex);
        byte[] TEMP = computeTemp(K, RAND, OPc);
        byte[] IN1  = buildIN1(SQN, AMF);
        byte[] OUT1 = xor(aesEncrypt(K, xorLastByte(xor(IN1, rotate(xor(TEMP, OPc), R1)), C1)), OPc);
        return bytesToHex(subarray(OUT1, 0, 8));
    }

    /**
     * Compute MAC-S (f1*) with AMF* = 0x0000 per TS 35.206.
     * Exposes the raw f1* function for direct spec-vector verification.
     *
     * @param rand 32 hex chars (16 bytes)
     * @param k    32 hex chars (16 bytes)
     * @param opc  32 hex chars (16 bytes)
     * @param sqn  48-bit sequence number
     * @return MAC-S as 16 hex chars (8 bytes)
     */
    public String computeF1Star(String rand, String k, String opc, long sqn) {
        requireHexLength(rand, 32, "rand");
        requireHexLength(k,    32, "k");
        requireHexLength(opc,  32, "opc");
        byte[] K    = hexToBytes(k);
        byte[] RAND = hexToBytes(rand);
        byte[] OPc  = hexToBytes(opc);
        byte[] SQN  = sqnToBytes(sqn);
        byte[] TEMP = computeTemp(K, RAND, OPc);
        return bytesToHex(computeMacS(K, TEMP, OPc, SQN));
    }

    public OptionalLong validateAutsAndRecoverSqn(String rand, String auts, String permanentKey, String opc) {
        if (auts == null || auts.isBlank() || auts.length() != 28) {
            return OptionalLong.empty();
        }

        requireHexLength(rand, 32, "rand");
        requireHexLength(permanentKey, 32, "permanentKey");
        requireHexLength(opc, 32, "opc");

        byte[] K = hexToBytes(permanentKey);
        byte[] RAND = hexToBytes(rand);
        byte[] OPc = hexToBytes(opc);
        byte[] autsBytes = hexToBytes(auts);
        byte[] concealedSqn = subarray(autsBytes, 0, 6);
        byte[] providedMacS = subarray(autsBytes, 6, 8);
        byte[] TEMP = computeTemp(K, RAND, OPc);
        byte[] akStar = computeAkStar(K, TEMP, OPc);
        byte[] recoveredSqn = xor(concealedSqn, akStar);
        byte[] expectedMacS = computeMacS(K, TEMP, OPc, recoveredSqn);
        if (!MessageDigest.isEqual(expectedMacS, providedMacS)) {
            return OptionalLong.empty();
        }

        return OptionalLong.of(bytesToSqn(recoveredSqn));
    }

    // ── 5G KDF (TS 33.501 Annex A) ────────────────────────────────────────────

    /** XRES* per TS 33.501 A.4: last 16 bytes of HMAC-SHA-256(CK||IK, S). */
    private byte[] deriveXresStar(byte[] ckIk, String snName, byte[] rand, byte[] res) {
        byte[] S = kdfInput((byte) 0x6B, snName.getBytes(StandardCharsets.UTF_8), rand, res);
        byte[] mac = hmacSha256(ckIk, S);
        return subarray(mac, mac.length - 16, 16);
    }

    /** KAUSF per TS 33.501 A.2: HMAC-SHA-256(CK||IK, S). */
    private byte[] deriveKausf(byte[] ckIk, String snName, byte[] sqnXorAk) {
        byte[] S = kdfInput((byte) 0x6A, snName.getBytes(StandardCharsets.UTF_8), sqnXorAk);
        return hmacSha256(ckIk, S);
    }

    /** Build KDF input string: FC || P0 || L0 || P1 || L1 || ... */
    private static byte[] kdfInput(byte fc, byte[]... params) {
        int total = 1;
        for (byte[] p : params) total += p.length + 2;
        byte[] result = new byte[total];
        int idx = 0;
        result[idx++] = fc;
        for (byte[] p : params) {
            System.arraycopy(p, 0, result, idx, p.length);
            idx += p.length;
            result[idx++] = (byte) (p.length >> 8);
            result[idx++] = (byte) (p.length & 0xFF);
        }
        return result;
    }

    // ── Crypto primitives ─────────────────────────────────────────────────────

    private static byte[] aesEncrypt(byte[] key, byte[] data) {
        try {
            Cipher cipher = Cipher.getInstance("AES/ECB/NoPadding");
            cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(key, "AES"));
            return cipher.doFinal(data);
        } catch (Exception e) {
            throw new IllegalStateException("AES encryption failed", e);
        }
    }

    private static byte[] computeTemp(byte[] key, byte[] rand, byte[] opC) {
        return aesEncrypt(key, xor(rand, opC));
    }

    private static byte[] computeAkStar(byte[] key, byte[] temp, byte[] opC) {
        byte[] out5 = xor(aesEncrypt(key, xorLastByte(rotate(xor(temp, opC), R5), C5)), opC);
        return subarray(out5, 0, 6);
    }

    private static byte[] computeMacS(byte[] key, byte[] temp, byte[] opC, byte[] sqn) {
        byte[] in1 = buildIN1(sqn, RESYNC_AMF);
        byte[] out1 = xor(aesEncrypt(key, xorLastByte(xor(in1, rotate(xor(temp, opC), R1)), C1)), opC);
        return subarray(out1, 8, 8);
    }

    private static byte[] hmacSha256(byte[] key, byte[] data) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            return mac.doFinal(data);
        } catch (Exception e) {
            throw new IllegalStateException("HMAC-SHA-256 failed", e);
        }
    }

    private static byte[] sha256Prefix(byte[] data, int prefixLen) {
        try {
            byte[] hash = MessageDigest.getInstance("SHA-256").digest(data);
            return subarray(hash, 0, prefixLen);
        } catch (Exception e) {
            throw new IllegalStateException("SHA-256 failed", e);
        }
    }

    // ── Byte-array helpers ────────────────────────────────────────────────────

    /** Left-rotate {@code block} by {@code n} bytes. */
    private static byte[] rotate(byte[] block, int n) {
        if (n == 0) return block.clone();
        int len = block.length;
        n = ((n % len) + len) % len;
        byte[] result = new byte[len];
        for (int i = 0; i < len; i++) result[i] = block[(i + n) % len];
        return result;
    }

    private static byte[] xor(byte[] a, byte[] b) {
        int len = Math.min(a.length, b.length);
        byte[] result = new byte[len];
        for (int i = 0; i < len; i++) result[i] = (byte) (a[i] ^ b[i]);
        return result;
    }

    /** XOR {@code constVal} into the last byte of a copy of {@code a}. */
    private static byte[] xorLastByte(byte[] a, int constVal) {
        byte[] result = a.clone();
        result[result.length - 1] ^= (byte) constVal;
        return result;
    }

    private static byte[] subarray(byte[] src, int offset, int length) {
        byte[] result = new byte[length];
        System.arraycopy(src, offset, result, 0, length);
        return result;
    }

    private static byte[] concat(byte[] a, byte[] b) {
        byte[] result = new byte[a.length + b.length];
        System.arraycopy(a, 0, result, 0, a.length);
        System.arraycopy(b, 0, result, a.length, b.length);
        return result;
    }

    private static byte[] buildIN1(byte[] sqn, byte[] amf) {
        byte[] result = new byte[16];
        System.arraycopy(sqn, 0, result,  0, 6);
        System.arraycopy(amf, 0, result,  6, 2);
        System.arraycopy(sqn, 0, result,  8, 6);
        System.arraycopy(amf, 0, result, 14, 2);
        return result;
    }

    private static byte[] sqnToBytes(long sqn) {
        byte[] result = new byte[6];
        for (int i = 5; i >= 0; i--) {
            result[i] = (byte) (sqn & 0xFF);
            sqn >>>= 8;
        }
        return result;
    }

    private static long bytesToSqn(byte[] sqnBytes) {
        long result = 0;
        for (byte sqnByte : sqnBytes) {
            result = (result << 8) | (sqnByte & 0xFFL);
        }
        return result;
    }

    public static byte[] hexToBytes(String hex) {
        if (hex == null || hex.isBlank()) {
            throw new IllegalArgumentException("hex value must not be blank");
        }

        int len = hex.length();
        if ((len & 1) != 0) {
            throw new IllegalArgumentException("hex value must have an even number of characters");
        }

        byte[] result = new byte[len / 2];
        for (int i = 0; i < len; i += 2) {
            int high = Character.digit(hex.charAt(i), 16);
            int low = Character.digit(hex.charAt(i + 1), 16);
            if (high < 0 || low < 0) {
                throw new IllegalArgumentException("hex value contains non-hex characters");
            }
            result[i / 2] = (byte) ((high << 4) | low);
        }
        return result;
    }

    private static void requireHexLength(String value, int expectedLength, String fieldName) {
        if (value == null || value.length() != expectedLength) {
            throw new IllegalArgumentException(fieldName + " must be exactly " + expectedLength + " hex characters");
        }
    }

    public static String bytesToHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) sb.append(String.format("%02x", b));
        return sb.toString();
    }
}