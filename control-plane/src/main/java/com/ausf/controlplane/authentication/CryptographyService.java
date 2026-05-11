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
     * Derive K_aut' from KAUSF for EAP-AKA' AT_MAC computation.
     * In this mock/test environment K_aut' = HMAC-SHA-256(KAUSF, "K_aut'")[0:32].
     *
     * @param kausf 64 hex chars (32 bytes)
     * @return 32-byte K_aut'
     */
    public byte[] deriveKautPrime(String kausf) {
        try {
            byte[] key   = Milenage.hexToBytes(kausf);
            byte[] label = "K_aut'".getBytes(StandardCharsets.UTF_8);
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            return mac.doFinal(label);
        } catch (Exception e) {
            throw new IllegalStateException("K_aut' derivation failed", e);
        }
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

    /**
     * Derive Ksorsaf from KAUSF per 3GPP TS 33.501 Annex A.18.
     * Ksorsaf = HMAC-SHA-256(KAUSF, FC=0x20 || snName || len(snName) || counterSoR || 0x00 0x02)
     *
     * @param kausf              64 hex chars (32 bytes)
     * @param servingNetworkName e.g. "5G:mnc001.mcc001.3gppnetwork.org"
     * @param counter            2-byte big-endian counter value
     * @return 64 hex chars (32 bytes)
     */
    public String deriveKsorsaf(String kausf, String servingNetworkName, byte[] counter) {
        return deriveProtectionKey(kausf, servingNetworkName, counter, (byte) 0x20);
    }

    /**
     * Derive Kupusaf from KAUSF per 3GPP TS 33.501 Annex A.19.
     * Kupusaf = HMAC-SHA-256(KAUSF, FC=0x21 || snName || len(snName) || counterUPU || 0x00 0x02)
     */
    public String deriveKupusaf(String kausf, String servingNetworkName, byte[] counter) {
        return deriveProtectionKey(kausf, servingNetworkName, counter, (byte) 0x21);
    }

    private String deriveProtectionKey(String kausf, String servingNetworkName, byte[] counter, byte fc) {
        try {
            byte[] key    = Milenage.hexToBytes(kausf);
            byte[] snName = servingNetworkName.getBytes(StandardCharsets.UTF_8);
            int snLen     = snName.length;
            // S = FC || P0(snName) || L0(2 bytes) || P1(counter,2 bytes) || L1(0x0002)
            byte[] S = new byte[1 + snLen + 2 + 2 + 2];
            int i = 0;
            S[i++] = fc;
            System.arraycopy(snName, 0, S, i, snLen);
            i += snLen;
            S[i++] = (byte) (snLen >> 8);
            S[i++] = (byte) (snLen & 0xFF);
            S[i++] = counter[0];
            S[i++] = counter[1];
            S[i++] = 0x00;
            S[i]   = 0x02;
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            return bytesToHex(mac.doFinal(S));
        } catch (Exception e) {
            throw new IllegalStateException("Protection key derivation failed", e);
        }
    }

    /**
     * Compute MAC-SoR or MAC-UPU per TS 33.501 A.18/A.19.
     * MAC = first 16 bytes of HMAC-SHA-256(derivedKey, counter || [header ||] protectionData || ackByte)
     *
     * @param derivedKeyHex 64 hex chars (Ksorsaf or Kupusaf)
     * @param counter       2-byte big-endian counter
     * @param protectionData hex-encoded container data (steeringContainer or upuData)
     * @param ackIndication whether acknowledgement is requested
     * @return 32 hex chars (16 bytes)
     */
    public String computeProtectionMAC(String derivedKeyHex, byte[] counter, String protectionData, boolean ackIndication) {
        return computeProtectionMAC(derivedKeyHex, counter, null, protectionData, ackIndication);
    }

    /**
     * Compute MAC-SoR or MAC-UPU with optional header per TS 33.501 A.18/A.19.
     * When {@code headerData} is non-null (sorHeader / upuHeader from TS 29.509 request body),
     * it is prepended to the HMAC input:
     * input = counter(2) || header || protectionData || ackByte
     *
     * @param derivedKeyHex  64 hex chars (Ksorsaf or Kupusaf)
     * @param counter        2-byte big-endian counter
     * @param headerData     hex-encoded optional header (sorHeader / upuHeader), or null
     * @param protectionData hex-encoded container data (steeringContainer or upuData)
     * @param ackIndication  whether acknowledgement is requested
     * @return 32 hex chars (16 bytes)
     */
    public String computeProtectionMAC(String derivedKeyHex, byte[] counter, String headerData,
                                       String protectionData, boolean ackIndication) {
        try {
            byte[] key    = Milenage.hexToBytes(derivedKeyHex);
            byte[] hdr    = (headerData != null && !headerData.isBlank()) ? Milenage.hexToBytes(headerData) : new byte[0];
            byte[] data   = Milenage.hexToBytes(protectionData);
            byte   ack    = ackIndication ? (byte) 0x01 : (byte) 0x00;
            // input = counter(2) || [header ||] data || ackByte
            byte[] input  = new byte[2 + hdr.length + data.length + 1];
            input[0] = counter[0];
            input[1] = counter[1];
            System.arraycopy(hdr, 0, input, 2, hdr.length);
            System.arraycopy(data, 0, input, 2 + hdr.length, data.length);
            input[input.length - 1] = ack;
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            byte[] fullMac = mac.doFinal(input);
            // Return first 16 bytes (128 bits)
            byte[] truncated = new byte[16];
            System.arraycopy(fullMac, 0, truncated, 0, 16);
            return bytesToHex(truncated);
        } catch (Exception e) {
            throw new IllegalStateException("Protection MAC computation failed", e);
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
