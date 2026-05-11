package com.ausf.controlplane.crypto;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.OptionalLong;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * TUAK algorithm following the 3GPP TS 35.231 family, using the raw
 * Keccak-f[1600] permutation and an already-derived 32-byte TOPc input.
 * 5G key derivations follow 3GPP TS 33.501 Annex A (identical to Milenage).
 */
public class Tuak {

    private static final byte[] ALGONAME = "TUAK1.0".getBytes(StandardCharsets.US_ASCII);
    private static final byte[] DEFAULT_AMF = {(byte) 0x80, (byte) 0x00};
    private static final int KECCAK_ITERATIONS = 1;
    private static final int LEN_MAC_BITS = 64;
    private static final int LEN_RES_BITS = 64;
    private static final int LEN_CK_BITS = 128;
    private static final int LEN_IK_BITS = 128;
    private static final byte[] TUAK_PADDING = buildTuakPadding();
    private static final byte[] TUAK_TRAILER = new byte[64];
    private static final long[] ROUND_CONSTANTS = {
        0x0000000000000001L, 0x0000000000008082L,
        0x800000000000808aL, 0x8000000080008000L,
        0x000000000000808bL, 0x0000000080000001L,
        0x8000000080008081L, 0x8000000000008009L,
        0x000000000000008aL, 0x0000000000000088L,
        0x0000000080008009L, 0x000000008000000aL,
        0x000000008000808bL, 0x800000000000008bL,
        0x8000000000008089L, 0x8000000000008003L,
        0x8000000000008002L, 0x8000000000000080L,
        0x000000000000800aL, 0x800000008000000aL,
        0x8000000080008081L, 0x8000000000008080L,
        0x0000000080000001L, 0x8000000080008008L
    };
    private static final int[][] ROTATION_OFFSETS = {
        {0, 36, 3, 41, 18},
        {1, 44, 10, 45, 2},
        {62, 6, 43, 15, 61},
        {28, 55, 25, 21, 56},
        {27, 20, 39, 8, 14}
    };

    /**
     * Full 5G TUAK vector.
     *
     * @param rand      RAND  – 32 hex chars (16 bytes)
     * @param autn      AUTN  – 32 hex chars: (SQN⊕AK)||AMF||MAC-A
     * @param xresStar  XRES* – 32 hex chars: 5G derived per TS 33.501 A.4
     * @param hxresStar HXRES*– 32 hex chars: SHA-256(RAND||XRES*)[0:16]
     * @param kausf     KAUSF – 64 hex chars: 5G derived per TS 33.501 A.2
     */
    public record TuakVector(
        String rand,
        String autn,
        String xresStar,
        String hxresStar,
        String kausf
    ) {}

    /**
     * Compute the full 5G AKA TUAK authentication vector.
     *
     * @param rand               16-byte RAND as 32 hex chars
     * @param permanentKey       K: 16-byte or 32-byte key as 32 or 64 hex chars
     * @param topc               TUAK TOPc: 32-byte value as 64 hex chars
     * @param sqn                48-bit sequence number
     * @param servingNetworkName e.g. "5G:mnc001.mcc001.3gppnetwork.org"
     */
    public TuakVector generateVector(
        String rand,
        String permanentKey,
        String topc,
        long sqn,
        String servingNetworkName
    ) {
        requireHexLength(rand, 32, "rand");
        requireSupportedKeyLength(permanentKey);
        requireHexLength(topc, 64, "topc");

        byte[] key = hexToBytes(permanentKey);
        byte[] randBytes = hexToBytes(rand);
        byte[] topcBytes = hexToBytes(topc);
        byte[] sqnBytes = sqnToBytes(sqn);

        byte[] macA = computeMac(key, randBytes, sqnBytes, DEFAULT_AMF, topcBytes, false);
        TuakF2345Outputs outputs = computeF2345(key, randBytes, topcBytes);

        // AUTN = (SQN ⊕ AK) || AMF || MAC-A
        byte[] sqnXorAk = xor(sqnBytes, outputs.ak());
        byte[] autn = concat(concat(sqnXorAk, DEFAULT_AMF), macA);

        // 5G derivations (TS 33.501 – identical KDF to Milenage path)
        byte[] ckIk = concat(outputs.ck(), outputs.ik());
        byte[] xresStar = deriveXresStar(ckIk, servingNetworkName, randBytes, outputs.res());
        byte[] hxresStar = sha256Prefix(concat(randBytes, xresStar), 16);
        byte[] kausf = deriveKausf(ckIk, servingNetworkName, sqnXorAk);

        return new TuakVector(
            rand,
            bytesToHex(autn),
            bytesToHex(xresStar),
            bytesToHex(hxresStar),
            bytesToHex(kausf)
        );
    }

    String deriveTopc(String permanentKey, String top) {
        requireSupportedKeyLength(permanentKey);
        requireHexLength(top, 64, "top");

        byte[] key = hexToBytes(permanentKey);
        byte[] topBytes = hexToBytes(top);
        byte[] state = concat(
            reverse(topBytes),
            new byte[] {instanceByte(key.length, 0x00, 0x01)},
            reverse(ALGONAME),
            new byte[24],
            reverseAndPadKey(key),
            TUAK_PADDING,
            TUAK_TRAILER
        );
        return bytesToHex(reverse(subarray(runKeccak(state), 0, 32)));
    }

    String generateMacA(String rand, String permanentKey, String topc, long sqn, String amf) {
        return generateMac(rand, permanentKey, topc, sqn, amf, false);
    }

    String generateMacS(String rand, String permanentKey, String topc, long sqn, String amf) {
        return generateMac(rand, permanentKey, topc, sqn, amf, true);
    }

    TuakF2345Vector generateF2345(String rand, String permanentKey, String topc) {
        requireHexLength(rand, 32, "rand");
        requireSupportedKeyLength(permanentKey);
        requireHexLength(topc, 64, "topc");

        TuakF2345Outputs outputs = computeF2345(hexToBytes(permanentKey), hexToBytes(rand), hexToBytes(topc));
        return new TuakF2345Vector(
            bytesToHex(outputs.res()),
            bytesToHex(outputs.ck()),
            bytesToHex(outputs.ik()),
            bytesToHex(outputs.ak())
        );
    }

    String generateAkStar(String rand, String permanentKey, String topc) {
        requireHexLength(rand, 32, "rand");
        requireSupportedKeyLength(permanentKey);
        requireHexLength(topc, 64, "topc");

        byte[] akStar = computeAkStar(hexToBytes(permanentKey), hexToBytes(rand), hexToBytes(topc));
        return bytesToHex(akStar);
    }

    // ── 5G KDF (TS 33.501 Annex A) ────────────────────────────────────────────

    private byte[] deriveXresStar(byte[] ckIk, String snName, byte[] rand, byte[] res) {
        byte[] S = kdfInput((byte) 0x6B, snName.getBytes(StandardCharsets.UTF_8), rand, res);
        byte[] mac = hmacSha256(ckIk, S);
        return subarray(mac, mac.length - 16, 16);
    }

    private byte[] deriveKausf(byte[] ckIk, String snName, byte[] sqnXorAk) {
        byte[] S = kdfInput((byte) 0x6A, snName.getBytes(StandardCharsets.UTF_8), sqnXorAk);
        return hmacSha256(ckIk, S);
    }

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

    record TuakF2345Vector(String res, String ck, String ik, String ak) {}

    private record TuakF2345Outputs(byte[] res, byte[] ck, byte[] ik, byte[] ak) {}

    // ── TUAK core functions ───────────────────────────────────────────────────

    private String generateMac(String rand, String permanentKey, String topc, long sqn, String amf, boolean starred) {
        requireHexLength(rand, 32, "rand");
        requireSupportedKeyLength(permanentKey);
        requireHexLength(topc, 64, "topc");
        requireHexLength(amf, 4, "amf");

        byte[] mac = computeMac(
            hexToBytes(permanentKey),
            hexToBytes(rand),
            sqnToBytes(sqn),
            hexToBytes(amf),
            hexToBytes(topc),
            starred
        );
        return bytesToHex(mac);
    }

    private byte[] computeMac(byte[] key, byte[] rand, byte[] sqn, byte[] amf, byte[] topc, boolean starred) {
        validateMacArgs(key, rand, sqn, amf, topc);

        int outputLength = switch (LEN_MAC_BITS) {
            case 64 -> 8;
            case 128 -> 16;
            case 256 -> 32;
            default -> throw new IllegalStateException("unsupported TUAK MAC length");
        };
        int baseInstance = switch (LEN_MAC_BITS) {
            case 64 -> starred ? 0x88 : 0x08;
            case 128 -> starred ? 0x90 : 0x10;
            case 256 -> starred ? 0xA0 : 0x20;
            default -> throw new IllegalStateException("unsupported TUAK MAC length");
        };

        byte[] state = concat(
            reverse(topc),
            new byte[] {instanceByte(key.length, baseInstance, baseInstance + 1)},
            reverse(ALGONAME),
            reverse(rand),
            reverse(amf),
            reverse(sqn),
            reverseAndPadKey(key),
            TUAK_PADDING,
            TUAK_TRAILER
        );
        return reverse(subarray(runKeccak(state), 0, outputLength));
    }

    private TuakF2345Outputs computeF2345(byte[] key, byte[] rand, byte[] topc) {
        validateF2345Args(key, rand, topc);

        int resLength = switch (LEN_RES_BITS) {
            case 32 -> 4;
            case 64 -> 8;
            case 128 -> 16;
            case 256 -> 32;
            default -> throw new IllegalStateException("unsupported TUAK RES length");
        };
        int ckLength = LEN_CK_BITS == 256 ? 32 : 16;
        int ikLength = LEN_IK_BITS == 256 ? 32 : 16;

        int instance = switch (LEN_RES_BITS) {
            case 32 -> 0x40;
            case 64 -> 0x48;
            case 128 -> 0x50;
            case 256 -> 0x60;
            default -> throw new IllegalStateException("unsupported TUAK RES length");
        };
        if (LEN_CK_BITS == 256) {
            instance += 0x04;
        }
        if (LEN_IK_BITS == 256) {
            instance += 0x02;
        }
        if (key.length == 32) {
            instance += 0x01;
        }

        byte[] state = concat(
            reverse(topc),
            new byte[] {(byte) instance},
            reverse(ALGONAME),
            reverse(rand),
            new byte[8],
            reverseAndPadKey(key),
            TUAK_PADDING,
            TUAK_TRAILER
        );
        byte[] output = runKeccak(state);
        return new TuakF2345Outputs(
            reverse(subarray(output, 0, resLength)),
            reverse(subarray(output, 32, ckLength)),
            reverse(subarray(output, 64, ikLength)),
            reverse(subarray(output, 96, 6))
        );
    }

    private byte[] computeAkStar(byte[] key, byte[] rand, byte[] topc) {
        validateF2345Args(key, rand, topc);

        byte[] state = concat(
            reverse(topc),
            new byte[] {instanceByte(key.length, 0xC0, 0xC1)},
            reverse(ALGONAME),
            reverse(rand),
            new byte[8],
            reverseAndPadKey(key),
            TUAK_PADDING,
            TUAK_TRAILER
        );
        return reverse(subarray(runKeccak(state), 96, 6));
    }

    // ── Crypto primitives ─────────────────────────────────────────────────────

    private byte[] runKeccak(byte[] state) {
        if (state.length != 200) {
            throw new IllegalArgumentException("TUAK state must be exactly 200 bytes");
        }

        byte[] output = state.clone();
        for (int iteration = 0; iteration < KECCAK_ITERATIONS; iteration++) {
            output = keccakF1600(output);
        }
        return output;
    }

    private byte[] sha256Prefix(byte[] data, int prefixLen) {
        try {
            byte[] hash = MessageDigest.getInstance("SHA-256").digest(data);
            return subarray(hash, 0, prefixLen);
        } catch (Exception e) {
            throw new IllegalStateException("SHA-256 failed", e);
        }
    }

    private byte[] hmacSha256(byte[] key, byte[] data) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            return mac.doFinal(data);
        } catch (Exception e) {
            throw new IllegalStateException("HMAC-SHA-256 failed", e);
        }
    }

    private static byte[] keccakF1600(byte[] input) {
        long[][] lanes = new long[5][5];
        for (int y = 0; y < 5; y++) {
            for (int x = 0; x < 5; x++) {
                lanes[x][y] = littleEndianToLong(input, 8 * (x + 5 * y));
            }
        }

        for (long roundConstant : ROUND_CONSTANTS) {
            long[] c = new long[5];
            for (int x = 0; x < 5; x++) {
                c[x] = lanes[x][0] ^ lanes[x][1] ^ lanes[x][2] ^ lanes[x][3] ^ lanes[x][4];
            }

            long[] d = new long[5];
            for (int x = 0; x < 5; x++) {
                d[x] = c[(x + 4) % 5] ^ Long.rotateLeft(c[(x + 1) % 5], 1);
            }
            for (int x = 0; x < 5; x++) {
                for (int y = 0; y < 5; y++) {
                    lanes[x][y] ^= d[x];
                }
            }

            long[][] b = new long[5][5];
            for (int x = 0; x < 5; x++) {
                for (int y = 0; y < 5; y++) {
                    b[y][(2 * x + 3 * y) % 5] = Long.rotateLeft(lanes[x][y], ROTATION_OFFSETS[x][y]);
                }
            }

            for (int x = 0; x < 5; x++) {
                for (int y = 0; y < 5; y++) {
                    lanes[x][y] = b[x][y] ^ ((~b[(x + 1) % 5][y]) & b[(x + 2) % 5][y]);
                }
            }

            lanes[0][0] ^= roundConstant;
        }

        byte[] output = new byte[200];
        for (int y = 0; y < 5; y++) {
            for (int x = 0; x < 5; x++) {
                longToLittleEndian(lanes[x][y], output, 8 * (x + 5 * y));
            }
        }
        return output;
    }

    // ── Byte-array helpers ────────────────────────────────────────────────────

    private static byte[] xor(byte[] a, byte[] b) {
        int len = Math.min(a.length, b.length);
        byte[] result = new byte[len];
        for (int i = 0; i < len; i++) result[i] = (byte) (a[i] ^ b[i]);
        return result;
    }

    private static byte[] reverse(byte[] input) {
        byte[] output = input.clone();
        for (int left = 0, right = output.length - 1; left < right; left++, right--) {
            byte current = output[left];
            output[left] = output[right];
            output[right] = current;
        }
        return output;
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

    private static byte[] concat(byte[]... arrays) {
        int totalLength = 0;
        for (byte[] array : arrays) {
            totalLength += array.length;
        }

        byte[] result = new byte[totalLength];
        int offset = 0;
        for (byte[] array : arrays) {
            System.arraycopy(array, 0, result, offset, array.length);
            offset += array.length;
        }
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

    private static byte[] hexToBytes(String hex) {
        int len = hex.length();
        byte[] result = new byte[len / 2];
        for (int i = 0; i < len; i += 2) {
            result[i / 2] = (byte) ((Character.digit(hex.charAt(i), 16) << 4)
                | Character.digit(hex.charAt(i + 1), 16));
        }
        return result;
    }

    private static long littleEndianToLong(byte[] input, int offset) {
        long result = 0;
        for (int index = 7; index >= 0; index--) {
            result = (result << 8) | (input[offset + index] & 0xFFL);
        }
        return result;
    }

    private static void longToLittleEndian(long value, byte[] output, int offset) {
        for (int index = 0; index < 8; index++) {
            output[offset + index] = (byte) (value & 0xFF);
            value >>>= 8;
        }
    }

    private static byte[] reverseAndPadKey(byte[] key) {
        return key.length == 16 ? concat(reverse(key), new byte[16]) : reverse(key);
    }

    private static byte instanceByte(int keyLength, int shortKeyValue, int longKeyValue) {
        return (byte) (keyLength == 32 ? longKeyValue : shortKeyValue);
    }

    private static byte[] buildTuakPadding() {
        byte[] padding = new byte[40];
        padding[0] = 0x1F;
        padding[39] = (byte) 0x80;
        return padding;
    }

    private static void requireHexLength(String value, int expectedLength, String fieldName) {
        if (value == null || value.length() != expectedLength) {
            throw new IllegalArgumentException(fieldName + " must be exactly " + expectedLength + " hex characters");
        }
    }

    private static void requireSupportedKeyLength(String value) {
        if (value == null || (value.length() != 32 && value.length() != 64)) {
            throw new IllegalArgumentException("permanentKey must be 32 or 64 hex characters for TUAK");
        }
    }

    private static void validateMacArgs(byte[] key, byte[] rand, byte[] sqn, byte[] amf, byte[] topc) {
        if ((key.length != 16 && key.length != 32) || rand.length != 16 || sqn.length != 6 || amf.length != 2 || topc.length != 32) {
            throw new IllegalArgumentException("invalid TUAK MAC inputs");
        }
    }

    private static void validateF2345Args(byte[] key, byte[] rand, byte[] topc) {
        if ((key.length != 16 && key.length != 32) || rand.length != 16 || topc.length != 32) {
            throw new IllegalArgumentException("invalid TUAK vector inputs");
        }
    }

    private static String bytesToHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) sb.append(String.format("%02x", b));
        return sb.toString();
    }

    // ── AUTS (resynchronization) ───────────────────────────────────────────────

    /**
     * Generate AUTS for a synchronization failure response (TS 35.231).
     * AUTS = (SQN ⊕ AK*) || MAC-S  where AMF* = 0x0000.
     *
     * @return 14-byte AUTS as 28 hex chars
     */
    public String generateAuts(String rand, String permanentKey, String topc, long sqn) {
        requireHexLength(rand, 32, "rand");
        requireSupportedKeyLength(permanentKey);
        requireHexLength(topc, 64, "topc");

        byte[] akStar = hexToBytes(generateAkStar(rand, permanentKey, topc));
        byte[] sqnBytes = sqnToBytes(sqn);
        byte[] sqnXorAkStar = xor(sqnBytes, akStar);
        // MAC-S uses AMF* = 0x0000 for resynchronization per TS 35.231
        byte[] macS = hexToBytes(generateMacS(rand, permanentKey, topc, sqn, "0000"));
        return bytesToHex(concat(sqnXorAkStar, macS));
    }

    /**
     * Verify AUTS MAC-S and recover the UE's SQN (TS 35.231).
     *
     * @return the recovered SQN, or empty if MAC-S verification fails
     */
    public OptionalLong validateAutsAndRecoverSqn(
        String rand, String auts, String permanentKey, String topc
    ) {
        if (auts == null || auts.isBlank() || auts.length() != 28) {
            return OptionalLong.empty();
        }
        requireHexLength(rand, 32, "rand");
        requireSupportedKeyLength(permanentKey);
        requireHexLength(topc, 64, "topc");

        byte[] autsBytes = hexToBytes(auts);
        byte[] sqnXorAkStar = subarray(autsBytes, 0, 6);
        byte[] providedMacS = subarray(autsBytes, 6, 8);

        byte[] akStar = hexToBytes(generateAkStar(rand, permanentKey, topc));
        byte[] recoveredSqnBytes = xor(sqnXorAkStar, akStar);
        long recoveredSqn = bytesToSqn(recoveredSqnBytes);

        byte[] expectedMacS = hexToBytes(generateMacS(rand, permanentKey, topc, recoveredSqn, "0000"));
        if (!MessageDigest.isEqual(expectedMacS, providedMacS)) {
            return OptionalLong.empty();
        }
        return OptionalLong.of(recoveredSqn);
    }

    private static long bytesToSqn(byte[] sqnBytes) {
        long result = 0;
        for (byte b : sqnBytes) {
            result = (result << 8) | (b & 0xFFL);
        }
        return result;
    }
}