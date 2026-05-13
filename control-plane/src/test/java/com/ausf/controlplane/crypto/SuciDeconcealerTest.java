package com.ausf.controlplane.crypto;

import static org.junit.jupiter.api.Assertions.*;

import java.math.BigInteger;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.interfaces.ECPrivateKey;
import java.security.interfaces.ECPublicKey;
import java.security.spec.ECGenParameterSpec;
import java.security.spec.ECPoint;
import java.security.spec.NamedParameterSpec;
import java.util.Arrays;
import javax.crypto.Cipher;
import javax.crypto.KeyAgreement;
import javax.crypto.Mac;
import javax.crypto.spec.IvParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import org.junit.jupiter.api.Test;

/**
 * Tests for SUCI de-concealment (SIDF) — TS 33.501 Annex C.3.
 * Profile A and B are tested via round-trip: encrypt MSIN with the home network
 * public key (UE side), then verify SuciDeconcealer returns the original MSIN.
 */
public class SuciDeconcealerTest {

    private static final String MSIN = "1234567891";

    // ── Scheme 0 (null-scheme) ────────────────────────────────────────────────

    @Test
    void nullSchemeReturnsSchemeOutputUnchanged() {
        SuciDeconcealer d = new SuciDeconcealer("", "");
        assertEquals(MSIN, d.deconcealment(0, MSIN));
    }

    // ── Error paths ───────────────────────────────────────────────────────────

    @Test
    void unsupportedSchemeIdThrowsIllegalArgumentException() {
        SuciDeconcealer d = new SuciDeconcealer("", "");
        assertThrows(IllegalArgumentException.class, () -> d.deconcealment(3, "deadbeef"));
    }

    @Test
    void profileAKeyNotConfiguredThrowsIllegalStateException() {
        SuciDeconcealer d = new SuciDeconcealer("", "");
        assertThrows(IllegalStateException.class, () -> d.deconcealment(1, "deadbeef"));
    }

    @Test
    void profileBKeyNotConfiguredThrowsIllegalStateException() {
        SuciDeconcealer d = new SuciDeconcealer("", "");
        assertThrows(IllegalStateException.class, () -> d.deconcealment(2, "deadbeef"));
    }

    // ── Profile A (X25519) ────────────────────────────────────────────────────

    @Test
    void profileARoundTrip() throws Exception {
        // Generate home network X25519 key pair.
        KeyPairGenerator kpg = KeyPairGenerator.getInstance("XDH");
        kpg.initialize(new NamedParameterSpec("X25519"));
        KeyPair homeKP = kpg.generateKeyPair();

        // Extract raw 32-byte private scalar from PKCS#8 DER (header = 16 bytes).
        byte[] pkcs8 = homeKP.getPrivate().getEncoded();
        String homePrivHex = bytesToHex(Arrays.copyOfRange(pkcs8, 16, 48));

        SuciDeconcealer deconcealer = new SuciDeconcealer(homePrivHex, "");
        String schemeOutput = encryptProfileA(homeKP.getPublic(), MSIN);

        assertEquals(MSIN, deconcealer.deconcealment(1, schemeOutput));
    }

    @Test
    void profileATamperedMacTagIsRejected() throws Exception {
        KeyPairGenerator kpg = KeyPairGenerator.getInstance("XDH");
        kpg.initialize(new NamedParameterSpec("X25519"));
        KeyPair homeKP = kpg.generateKeyPair();

        byte[] pkcs8 = homeKP.getPrivate().getEncoded();
        SuciDeconcealer deconcealer = new SuciDeconcealer(bytesToHex(Arrays.copyOfRange(pkcs8, 16, 48)), "");

        byte[] raw = hexToBytes(encryptProfileA(homeKP.getPublic(), MSIN));
        raw[raw.length - 1] ^= 0xFF; // corrupt the last byte of the MAC tag

        assertThrows(SecurityException.class, () -> deconcealer.deconcealment(1, bytesToHex(raw)));
    }

    // ── Profile B (P-256) ─────────────────────────────────────────────────────

    @Test
    void profileBRoundTrip() throws Exception {
        // Generate home network P-256 key pair.
        KeyPairGenerator kpg = KeyPairGenerator.getInstance("EC");
        kpg.initialize(new ECGenParameterSpec("secp256r1"));
        KeyPair homeKP = kpg.generateKeyPair();

        // Extract raw 32-byte private scalar from ECPrivateKey.
        byte[] dBytes = toUnsigned32Bytes(((ECPrivateKey) homeKP.getPrivate()).getS());
        SuciDeconcealer deconcealer = new SuciDeconcealer("", bytesToHex(dBytes));
        String schemeOutput = encryptProfileB(homeKP.getPublic(), MSIN);

        assertEquals(MSIN, deconcealer.deconcealment(2, schemeOutput));
    }

    @Test
    void profileBTamperedMacTagIsRejected() throws Exception {
        KeyPairGenerator kpg = KeyPairGenerator.getInstance("EC");
        kpg.initialize(new ECGenParameterSpec("secp256r1"));
        KeyPair homeKP = kpg.generateKeyPair();

        byte[] dBytes = toUnsigned32Bytes(((ECPrivateKey) homeKP.getPrivate()).getS());
        SuciDeconcealer deconcealer = new SuciDeconcealer("", bytesToHex(dBytes));

        byte[] raw = hexToBytes(encryptProfileB(homeKP.getPublic(), MSIN));
        raw[raw.length - 1] ^= 0xFF; // corrupt the last byte of the MAC tag

        assertThrows(SecurityException.class, () -> deconcealer.deconcealment(2, bytesToHex(raw)));
    }

    // ── SUCI encryption helpers (UE side) ────────────────────────────────────

    /**
     * Encrypt MSIN using Profile A (X25519 + AES-128-CTR + HMAC-SHA-256).
     * Output: ephPubKey(32) || AES-CTR(MSIN-BCD) || HMAC(8)
     */
    private static String encryptProfileA(java.security.PublicKey homePubKey, String msin) throws Exception {
        KeyPairGenerator kpg = KeyPairGenerator.getInstance("XDH");
        kpg.initialize(new NamedParameterSpec("X25519"));
        KeyPair ephKP = kpg.generateKeyPair();

        // Raw X25519 public key bytes: SubjectPublicKeyInfo header = 12 bytes.
        byte[] ephPubRaw = Arrays.copyOfRange(ephKP.getPublic().getEncoded(), 12, 44);

        KeyAgreement ka = KeyAgreement.getInstance("XDH");
        ka.init(ephKP.getPrivate());
        ka.doPhase(homePubKey, true);
        byte[] sharedSecret = ka.generateSecret();

        return buildSuciOutput(sharedSecret, ephPubRaw, msin);
    }

    /**
     * Encrypt MSIN using Profile B (P-256 + AES-128-CTR + HMAC-SHA-256).
     * Output: ephCompressed(33) || AES-CTR(MSIN-BCD) || HMAC(8)
     */
    private static String encryptProfileB(java.security.PublicKey homePubKey, String msin) throws Exception {
        KeyPairGenerator kpg = KeyPairGenerator.getInstance("EC");
        kpg.initialize(new ECGenParameterSpec("secp256r1"));
        KeyPair ephKP = kpg.generateKeyPair();

        ECPublicKey ephPub = (ECPublicKey) ephKP.getPublic();
        ECPoint pt = ephPub.getW();
        byte prefix = pt.getAffineY().testBit(0) ? (byte) 0x03 : (byte) 0x02;
        byte[] ephX = toUnsigned32Bytes(pt.getAffineX());
        byte[] ephCompressed = new byte[33];
        ephCompressed[0] = prefix;
        System.arraycopy(ephX, 0, ephCompressed, 1, 32);

        KeyAgreement ka = KeyAgreement.getInstance("ECDH");
        ka.init(ephKP.getPrivate());
        ka.doPhase(homePubKey, true);
        byte[] sharedSecret = ka.generateSecret();

        return buildSuciOutput(sharedSecret, ephCompressed, msin);
    }

    /** Common ECIES output construction: HKDF → AES-CTR → HMAC → concatenate. */
    private static String buildSuciOutput(byte[] sharedSecret, byte[] saltBytes, String msin) throws Exception {
        byte[] okm = hkdfSha256(sharedSecret, saltBytes, new byte[0], 32);
        byte[] encKey = Arrays.copyOf(okm, 16);
        byte[] macKey = Arrays.copyOfRange(okm, 16, 32);

        byte[] plaintext = stringToBcd(msin);

        Cipher cipher = Cipher.getInstance("AES/CTR/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(encKey, "AES"), new IvParameterSpec(new byte[16]));
        byte[] ciphertext = cipher.doFinal(plaintext);

        Mac mac = Mac.getInstance("HmacSHA256");
        mac.init(new SecretKeySpec(macKey, "HmacSHA256"));
        byte[] tag = Arrays.copyOf(mac.doFinal(ciphertext), 8);

        byte[] result = new byte[saltBytes.length + ciphertext.length + 8];
        System.arraycopy(saltBytes, 0, result, 0, saltBytes.length);
        System.arraycopy(ciphertext, 0, result, saltBytes.length, ciphertext.length);
        System.arraycopy(tag, 0, result, saltBytes.length + ciphertext.length, 8);
        return bytesToHex(result);
    }

    // ── Crypto primitives ─────────────────────────────────────────────────────

    private static byte[] hkdfSha256(byte[] ikm, byte[] salt, byte[] info, int length) throws Exception {
        byte[] effectiveSalt = (salt == null || salt.length == 0) ? new byte[32] : salt;
        byte[] prk = hmacSha256(effectiveSalt, ikm);
        int n = (int) Math.ceil((double) length / 32);
        byte[] okm = new byte[n * 32];
        byte[] t = new byte[0];
        for (int i = 1; i <= n; i++) {
            byte[] input = new byte[t.length + info.length + 1];
            System.arraycopy(t, 0, input, 0, t.length);
            System.arraycopy(info, 0, input, t.length, info.length);
            input[input.length - 1] = (byte) i;
            t = hmacSha256(prk, input);
            System.arraycopy(t, 0, okm, (i - 1) * 32, 32);
        }
        return Arrays.copyOf(okm, length);
    }

    private static byte[] hmacSha256(byte[] key, byte[] data) throws Exception {
        Mac mac = Mac.getInstance("HmacSHA256");
        mac.init(new SecretKeySpec(key, "HmacSHA256"));
        return mac.doFinal(data);
    }

    /**
     * Encode decimal digit string as packed big-endian BCD.
     * "1234" → {0x12, 0x34}. Odd-length strings get a trailing 0xF nibble.
     */
    private static byte[] stringToBcd(String digits) {
        int len = (digits.length() + 1) / 2;
        byte[] result = new byte[len];
        for (int i = 0; i < digits.length(); i++) {
            int nibble = digits.charAt(i) - '0';
            if ((i & 1) == 0) {
                result[i / 2] = (byte) (nibble << 4);
            } else {
                result[i / 2] |= (byte) nibble;
            }
        }
        if ((digits.length() & 1) != 0) {
            result[len - 1] |= 0x0F;
        }
        return result;
    }

    // ── Byte utilities ────────────────────────────────────────────────────────

    /** Convert BigInteger to a zero-padded big-endian 32-byte array. */
    private static byte[] toUnsigned32Bytes(BigInteger value) {
        byte[] raw = value.toByteArray();
        if (raw.length == 32) return raw;
        byte[] result = new byte[32];
        int srcOffset = Math.max(0, raw.length - 32);
        System.arraycopy(raw, srcOffset, result, 32 - (raw.length - srcOffset), raw.length - srcOffset);
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

    private static String bytesToHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) sb.append(String.format("%02x", b & 0xFF));
        return sb.toString();
    }
}
