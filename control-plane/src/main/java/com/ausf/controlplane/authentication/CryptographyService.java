package com.ausf.controlplane.authentication;

import com.ausf.controlplane.crypto.Milenage;
import com.ausf.controlplane.crypto.Tuak;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.util.OptionalLong;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import org.springframework.stereotype.Service;

@Service
public class CryptographyService {
    private final Milenage milenage;
    private final Tuak tuak;
    private final SecureRandom secureRandom = new SecureRandom();

    public CryptographyService() {
        this.milenage = new Milenage();
        this.tuak = new Tuak();
    }

    public String generateRandomHex(int length) {
        byte[] bytes = new byte[length / 2];
        secureRandom.nextBytes(bytes);
        return bytesToHex(bytes);
    }

    public boolean verifyAuthentication(String expectedValue, String candidateValue) {
        return expectedValue != null && expectedValue.equalsIgnoreCase(candidateValue);
    }

    public String digestHex(String value) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(value.getBytes(StandardCharsets.UTF_8));
            return bytesToHex(hash);
        } catch (Exception exception) {
            throw new IllegalStateException("failed to calculate digest", exception);
        }
    }

    /**
     * Generate a full 5G AKA Milenage authentication vector (AES-based, TS 35.206 / TS 33.501).
     */
    public Milenage.MilenageVector generateMilenageVector(
        String rand, String permanentKey, String opc, long sqn, String servingNetworkName
    ) {
        return milenage.generateVector(rand, permanentKey, opc, sqn, servingNetworkName);
    }

    /**
     * Generate a full 5G AKA TUAK authentication vector (Keccak-f[1600]-based, TS 35.231 / TS 33.501).
     */
    public Tuak.TuakVector generateTuakVector(
        String rand, String permanentKey, String topc, long sqn, String servingNetworkName
    ) {
        return tuak.generateVector(rand, permanentKey, topc, sqn, servingNetworkName);
    }

    public String generateMilenageAuts(String rand, String permanentKey, String opc, long sqn) {
        return milenage.generateAuts(rand, permanentKey, opc, sqn);
    }

    public OptionalLong validateMilenageAuts(String rand, String auts, String permanentKey, String opc) {
        return milenage.validateAutsAndRecoverSqn(rand, auts, permanentKey, opc);
    }

    /**
     * Derive KSEAF from KAUSF per 3GPP TS 33.501 clause A.6.
     * KSEAF = HMAC-SHA-256(KAUSF, 0x6C || snName || len(snName))
     */
    public String deriveKseaf(String kausf, String servingNetworkName) {
        try {
            byte[] key = Milenage.hexToBytes(kausf);
            byte[] snName = servingNetworkName.getBytes(StandardCharsets.UTF_8);
            byte[] S = new byte[1 + snName.length + 2];
            S[0] = 0x6C;
            System.arraycopy(snName, 0, S, 1, snName.length);
            S[1 + snName.length]     = (byte) (snName.length >> 8);
            S[1 + snName.length + 1] = (byte) (snName.length & 0xFF);
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            return bytesToHex(mac.doFinal(S));
        } catch (Exception e) {
            throw new IllegalStateException("KSEAF derivation failed", e);
        }
    }

    private String bytesToHex(byte[] bytes) {
        StringBuilder builder = new StringBuilder();
        for (byte current : bytes) {
            builder.append(String.format("%02x", current));
        }
        return builder.toString();
    }
}
