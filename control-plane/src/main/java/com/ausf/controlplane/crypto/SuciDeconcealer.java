package com.ausf.controlplane.crypto;

import java.math.BigInteger;
import java.security.AlgorithmParameters;
import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.interfaces.ECPublicKey;
import java.security.spec.ECFieldFp;
import java.security.spec.ECGenParameterSpec;
import java.security.spec.ECParameterSpec;
import java.security.spec.ECPoint;
import java.security.spec.ECPrivateKeySpec;
import java.security.spec.ECPublicKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.util.Arrays;
import javax.crypto.Cipher;
import javax.crypto.KeyAgreement;
import javax.crypto.Mac;
import javax.crypto.spec.IvParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

/**
 * SUCI de-concealment (SIDF) per 3GPP TS 33.501 Annex C.3.
 *
 * <p>Supported protection schemes:
 * <ul>
 *   <li>0 — Null-scheme: schemeOutput is the plaintext MSIN (decimal string). No crypto required.
 *   <li>1 — Profile A: ECIES with X25519 + AES-128-CTR + HMAC-SHA-256 (TS 33.501 C.3.4.1).
 *   <li>2 — Profile B: ECIES with P-256 + AES-128-CTR + HMAC-SHA-256 (TS 33.501 C.3.4.2).
 * </ul>
 *
 * <p>Home network private keys are supplied as raw 32-byte hex strings via environment variables:
 * {@code AUSF_SIDF_PROFILE_A_PRIVATE_KEY} and {@code AUSF_SIDF_PROFILE_B_PRIVATE_KEY}.
 */
@Component
public class SuciDeconcealer {

    // DER SubjectPublicKeyInfo header for X25519 (OID 1.3.101.110, RFC 8410).
    // Full encoding: SEQUENCE { SEQUENCE { OID X25519 } BIT STRING { 0x00 [32 bytes] } }
    private static final byte[] X25519_SPKI_HEADER = {
        0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x6e, 0x03, 0x21, 0x00
    };

    // DER PKCS#8 PrivateKeyInfo header for X25519 (RFC 8410).
    // SEQUENCE { INTEGER 0 SEQUENCE { OID X25519 } OCTET STRING { OCTET STRING { [32 bytes] } } }
    private static final byte[] X25519_PKCS8_HEADER = {
        0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x6e, 0x04, 0x22, 0x04, 0x20
    };

    private static final int PROFILE_A_EPH_KEY_BYTES = 32;
    private static final int PROFILE_B_EPH_KEY_BYTES = 33; // compressed P-256 point
    private static final int MAC_TAG_BYTES            = 8;
    private static final int ENC_KEY_BYTES            = 16;
    private static final int MAC_KEY_BYTES            = 16;

    private final String profileAPrivateKeyHex;
    private final String profileBPrivateKeyHex;

    public SuciDeconcealer(
        @Value("${ausf.sidf.profile-a-private-key:}") String profileAPrivateKeyHex,
        @Value("${ausf.sidf.profile-b-private-key:}") String profileBPrivateKeyHex
    ) {
        this.profileAPrivateKeyHex = profileAPrivateKeyHex == null ? "" : profileAPrivateKeyHex.trim();
        this.profileBPrivateKeyHex = profileBPrivateKeyHex == null ? "" : profileBPrivateKeyHex.trim();
    }

    /**
     * De-conceal a SUCI scheme output and return the plaintext MSIN decimal string.
     *
     * @param protectionSchemeId 0=null-scheme, 1=Profile A (X25519), 2=Profile B (P-256)
     * @param schemeOutput       the hex-encoded (Profile A/B) or decimal-string (null-scheme) scheme output
     * @return the MSIN as a decimal digit string
     */
    public String deconcealment(int protectionSchemeId, String schemeOutput) {
        return switch (protectionSchemeId) {
            case 0 -> schemeOutput;
            case 1 -> {
                if (profileAPrivateKeyHex.isBlank()) {
                    throw new IllegalStateException(
                        "SUCI Profile A de-concealment requested but ausf.sidf.profile-a-private-key is not configured");
                }
                yield deconcealProfileA(schemeOutput, profileAPrivateKeyHex);
            }
            case 2 -> {
                if (profileBPrivateKeyHex.isBlank()) {
                    throw new IllegalStateException(
                        "SUCI Profile B de-concealment requested but ausf.sidf.profile-b-private-key is not configured");
                }
                yield deconcealProfileB(schemeOutput, profileBPrivateKeyHex);
            }
            default -> throw new IllegalArgumentException(
                "Unsupported SUCI protection scheme ID: " + protectionSchemeId);
        };
    }

    // ── Profile A: X25519 ────────────────────────────────────────────────────

    private String deconcealProfileA(String schemeOutput, String privKeyHex) {
        byte[] raw = hexToBytes(schemeOutput);
        if (raw.length < PROFILE_A_EPH_KEY_BYTES + MAC_TAG_BYTES) {
            throw new IllegalArgumentException(
                "SUCI Profile A scheme output too short: " + raw.length + " bytes");
        }

        byte[] ephPubKeyBytes = Arrays.copyOf(raw, PROFILE_A_EPH_KEY_BYTES);
        byte[] ciphertext     = Arrays.copyOfRange(raw, PROFILE_A_EPH_KEY_BYTES, raw.length - MAC_TAG_BYTES);
        byte[] tag            = Arrays.copyOfRange(raw, raw.length - MAC_TAG_BYTES, raw.length);

        try {
            // Load home-network X25519 private key via PKCS#8 DER encoding.
            byte[] privRaw  = hexToBytes(privKeyHex);
            byte[] pkcs8Der = concat(X25519_PKCS8_HEADER, privRaw);
            PrivateKey homePrivKey = KeyFactory.getInstance("XDH")
                .generatePrivate(new java.security.spec.PKCS8EncodedKeySpec(pkcs8Der));

            // Load ephemeral X25519 public key via SubjectPublicKeyInfo DER encoding.
            byte[] spkiDer = concat(X25519_SPKI_HEADER, ephPubKeyBytes);
            PublicKey ephPubKey = KeyFactory.getInstance("XDH")
                .generatePublic(new X509EncodedKeySpec(spkiDer));

            // ECDH shared secret.
            KeyAgreement ka = KeyAgreement.getInstance("XDH");
            ka.init(homePrivKey);
            ka.doPhase(ephPubKey, true);
            byte[] sharedSecret = ka.generateSecret();

            // HKDF-SHA-256(ikm=sharedSecret, salt=ephPubKeyBytes, info="") → 32 bytes.
            byte[] okm    = hkdfSha256(sharedSecret, ephPubKeyBytes, new byte[0], ENC_KEY_BYTES + MAC_KEY_BYTES);
            byte[] encKey = Arrays.copyOf(okm, ENC_KEY_BYTES);
            byte[] macKey = Arrays.copyOfRange(okm, ENC_KEY_BYTES, ENC_KEY_BYTES + MAC_KEY_BYTES);

            // Verify MAC tag over ciphertext.
            byte[] expectedTag = Arrays.copyOf(hmacSha256(macKey, ciphertext), MAC_TAG_BYTES);
            if (!MessageDigest.isEqual(expectedTag, tag)) {
                throw new SecurityException("SUCI Profile A MAC verification failed");
            }

            // Decrypt AES-128-CTR (IV = all zeros, counter = 0).
            byte[] msinBytes = aesCtrDecrypt(encKey, ciphertext);
            return bcdToString(msinBytes);
        } catch (GeneralSecurityException e) {
            throw new IllegalStateException("SUCI Profile A de-concealment failed", e);
        }
    }

    // ── Profile B: P-256 ─────────────────────────────────────────────────────

    private String deconcealProfileB(String schemeOutput, String privKeyHex) {
        byte[] raw = hexToBytes(schemeOutput);
        if (raw.length < PROFILE_B_EPH_KEY_BYTES + MAC_TAG_BYTES) {
            throw new IllegalArgumentException(
                "SUCI Profile B scheme output too short: " + raw.length + " bytes");
        }

        byte[] ephCompressed = Arrays.copyOf(raw, PROFILE_B_EPH_KEY_BYTES);
        byte[] ciphertext    = Arrays.copyOfRange(raw, PROFILE_B_EPH_KEY_BYTES, raw.length - MAC_TAG_BYTES);
        byte[] tag           = Arrays.copyOfRange(raw, raw.length - MAC_TAG_BYTES, raw.length);

        try {
            ECParameterSpec p256 = getP256Params();
            KeyFactory kf = KeyFactory.getInstance("EC");

            // Load home-network P-256 private key from raw 32-byte big-endian scalar.
            BigInteger privScalar = new BigInteger(1, hexToBytes(privKeyHex));
            PrivateKey homePrivKey = kf.generatePrivate(new ECPrivateKeySpec(privScalar, p256));

            // Decompress and load ephemeral P-256 public key.
            ECPoint ephPoint = decompressP256Point(ephCompressed[0], Arrays.copyOfRange(ephCompressed, 1, 33), p256);
            PublicKey ephPubKey = kf.generatePublic(new ECPublicKeySpec(ephPoint, p256));

            // ECDH shared secret.
            KeyAgreement ka = KeyAgreement.getInstance("ECDH");
            ka.init(homePrivKey);
            ka.doPhase(ephPubKey, true);
            byte[] sharedSecret = ka.generateSecret();

            // HKDF-SHA-256(ikm=sharedSecret, salt=ephCompressed, info="") → 32 bytes.
            byte[] okm    = hkdfSha256(sharedSecret, ephCompressed, new byte[0], ENC_KEY_BYTES + MAC_KEY_BYTES);
            byte[] encKey = Arrays.copyOf(okm, ENC_KEY_BYTES);
            byte[] macKey = Arrays.copyOfRange(okm, ENC_KEY_BYTES, ENC_KEY_BYTES + MAC_KEY_BYTES);

            // Verify MAC.
            byte[] expectedTag = Arrays.copyOf(hmacSha256(macKey, ciphertext), MAC_TAG_BYTES);
            if (!MessageDigest.isEqual(expectedTag, tag)) {
                throw new SecurityException("SUCI Profile B MAC verification failed");
            }

            // Decrypt.
            byte[] msinBytes = aesCtrDecrypt(encKey, ciphertext);
            return bcdToString(msinBytes);
        } catch (GeneralSecurityException e) {
            throw new IllegalStateException("SUCI Profile B de-concealment failed", e);
        }
    }

    // ── Crypto helpers ────────────────────────────────────────────────────────

    /**
     * Manual HKDF-SHA-256 (RFC 5869): extract then expand.
     */
    private byte[] hkdfSha256(byte[] ikm, byte[] salt, byte[] info, int length) throws GeneralSecurityException {
        // Extract: PRK = HMAC-SHA-256(salt, IKM)
        byte[] effectiveSalt = (salt == null || salt.length == 0) ? new byte[32] : salt;
        byte[] prk = hmacSha256(effectiveSalt, ikm);

        // Expand: T(1) = HMAC-SHA-256(PRK, info || 0x01), ...
        int n = (int) Math.ceil((double) length / 32);
        byte[] okm = new byte[n * 32];
        byte[] t   = new byte[0];
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

    private byte[] hmacSha256(byte[] key, byte[] data) throws GeneralSecurityException {
        Mac mac = Mac.getInstance("HmacSHA256");
        mac.init(new SecretKeySpec(key, "HmacSHA256"));
        return mac.doFinal(data);
    }

    private byte[] aesCtrDecrypt(byte[] key, byte[] ciphertext) throws GeneralSecurityException {
        Cipher cipher = Cipher.getInstance("AES/CTR/NoPadding");
        byte[] iv = new byte[16]; // all zeros
        cipher.init(Cipher.DECRYPT_MODE, new SecretKeySpec(key, "AES"), new IvParameterSpec(iv));
        return cipher.doFinal(ciphertext);
    }

    /**
     * Decompress a P-256 public key point from the 33-byte compressed encoding.
     * Uses y² = x³ + ax + b (mod p) and the parity prefix (0x02=even, 0x03=odd).
     */
    private ECPoint decompressP256Point(byte prefix, byte[] xBytes, ECParameterSpec params) {
        ECFieldFp field = (ECFieldFp) params.getCurve().getField();
        BigInteger p = field.getP();
        BigInteger a = params.getCurve().getA();
        BigInteger b = params.getCurve().getB();
        BigInteger x = new BigInteger(1, xBytes);

        // y² = x³ + ax + b (mod p)
        BigInteger y2 = x.pow(3).add(a.multiply(x)).add(b).mod(p);

        // Modular square root: since p ≡ 3 (mod 4) for P-256, y = y²^((p+1)/4) mod p.
        BigInteger y = y2.modPow(p.add(BigInteger.ONE).shiftRight(2), p);

        // Select y with the correct parity (prefix 0x02=even, 0x03=odd).
        boolean prefixOdd = ((prefix & 1) == 1);
        if (prefixOdd != y.testBit(0)) {
            y = p.subtract(y);
        }
        return new ECPoint(x, y);
    }

    /** Obtain P-256 (secp256r1) ECParameterSpec from the JDK. */
    private ECParameterSpec getP256Params() throws GeneralSecurityException {
        AlgorithmParameters ap = AlgorithmParameters.getInstance("EC");
        ap.init(new ECGenParameterSpec("secp256r1"));
        return ap.getParameterSpec(ECParameterSpec.class);
    }

    /**
     * Convert packed BCD bytes to a decimal string.
     * Each nibble is one decimal digit; 0xF nibble = padding/stop.
     */
    private String bcdToString(byte[] bcd) {
        StringBuilder sb = new StringBuilder();
        for (byte b : bcd) {
            int hi = (b >> 4) & 0x0F;
            int lo = b & 0x0F;
            if (hi == 0xF) break;
            sb.append((char) ('0' + hi));
            if (lo == 0xF) break;
            sb.append((char) ('0' + lo));
        }
        return sb.toString();
    }

    private byte[] concat(byte[] a, byte[] b) {
        byte[] result = new byte[a.length + b.length];
        System.arraycopy(a, 0, result, 0, a.length);
        System.arraycopy(b, 0, result, a.length, b.length);
        return result;
    }

    public static byte[] hexToBytes(String hex) {
        if (hex == null || hex.isBlank()) return new byte[0];
        String h = hex.trim().toLowerCase();
        if ((h.length() & 1) != 0) h = "0" + h;
        byte[] result = new byte[h.length() / 2];
        for (int i = 0; i < result.length; i++) {
            result[i] = (byte) Integer.parseInt(h.substring(i * 2, i * 2 + 2), 16);
        }
        return result;
    }
}
