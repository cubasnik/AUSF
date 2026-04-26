package com.ausf.controlplane.authentication;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import org.springframework.stereotype.Service;

@Service
public class CryptographyService {
    private final SecureRandom secureRandom = new SecureRandom();

    public String generateRandomHex(int length) {
        byte[] bytes = new byte[length / 2];
        secureRandom.nextBytes(bytes);
        return bytesToHex(bytes);
    }

    public boolean verifyAuthentication(String expectedValue, String candidateValue) {
        return expectedValue != null && expectedValue.equalsIgnoreCase(candidateValue);
    }

    public String deriveExpectedResponse(String rand, String autn) {
        return digestHex(rand + autn).substring(0, 32);
    }

    public String deriveKey(String rand, String keyType) {
        return digestHex(rand + keyType);
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

    private String bytesToHex(byte[] bytes) {
        StringBuilder builder = new StringBuilder();
        for (byte current : bytes) {
            builder.append(String.format("%02x", current));
        }
        return builder.toString();
    }
}
