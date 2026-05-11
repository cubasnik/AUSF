package com.ausf.controlplane.eap;

import java.util.Arrays;
import java.util.Base64;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.security.SecureRandom;

/**
 * RFC 4187 / RFC 5448 binary EAP-AKA' packet encoder and decoder.
 *
 * <p>All encode methods return Base64url-encoded (no padding) strings suitable
 * for JSON transport per TS 29.509.  All decode methods accept Base64url input.
 *
 * <p>AT_MAC is computed using HMAC-SHA-256(K_aut', packet-with-mac-zeroed),
 * taking the first 16 bytes of output (RFC 5448 §3.3).
 */
public final class EapPacket {

    // EAP codes (RFC 3748 §4)
    private static final byte CODE_REQUEST  = 0x01;
    private static final byte CODE_RESPONSE = 0x02;
    public  static final byte CODE_FAILURE  = 0x04;

    // EAP method type: EAP-AKA' (RFC 5448)
    private static final byte TYPE_EAP_AKA_PRIME = 50; // 0x32

    // EAP-AKA subtypes (RFC 4187 §11)
    public static final byte SUBTYPE_AKA_CHALLENGE         = 0x01;
    public static final byte SUBTYPE_AKA_SYNC_FAILURE      = 0x04;
    public static final byte SUBTYPE_AKA_REAUTHENTICATION  = 0x0D;

    // Attribute types (RFC 4187 §11)
    private static final byte AT_RAND      = 0x01;
    private static final byte AT_AUTN      = 0x02;
    private static final byte AT_RES       = 0x03;
    private static final byte AT_AUTS      = 0x10; // 16
    private static final byte AT_MAC       = 0x0B; // 11
    private static final byte AT_KDF       = 0x18; // 24  (RFC 5448)
    private static final byte AT_KDF_INPUT = 0x17; // 23  (RFC 5448)
    private static final byte AT_NONCE_S   = 0x15; // 21

    // AT_KDF value: PRF'(HMAC-SHA-256) per RFC 5448 §3.2
    private static final byte KDF_PRF_HMAC_SHA256 = 0x01;

    // AUTS is always 14 bytes in TS 33.102
    private static final int AUTS_LENGTH = 14;

    private static final SecureRandom RANDOM = new SecureRandom();

    private EapPacket() {}

    // -----------------------------------------------------------------------
    // Encoding
    // -----------------------------------------------------------------------

    /**
     * Build a binary EAP-Request/AKA'-Challenge packet and return its
     * Base64url (no-padding) encoding.
     *
     * <p>Attributes included: AT_RAND, AT_AUTN, AT_KDF, AT_KDF_INPUT, AT_MAC.
     *
     * @param rand               16-byte RAND value
     * @param autn               16-byte AUTN value
     * @param servingNetworkName serving network name for AT_KDF_INPUT
     * @param kAutPrime          32-byte K_aut' for AT_MAC computation
     */
    public static String buildChallenge(byte[] rand, byte[] autn, String servingNetworkName, byte[] kAutPrime) {
        byte id = randomByte();
        byte[] packet = assembleChallengePacket(CODE_REQUEST, id, rand, autn, servingNetworkName, kAutPrime);
        return base64url(packet);
    }

    /**
     * Build a binary EAP-Response/AKA'-Challenge packet (as sent by the UE).
     * Used in unit tests and by test clients acting as the UE.
     *
     * @param identifier EAP identifier — must match the corresponding Request
     * @param resStar    RES* bytes (typically 16 bytes)
     * @param kAutPrime  32-byte K_aut'
     */
    public static String buildChallengeResponse(byte identifier, byte[] resStar, byte[] kAutPrime) {
        int resLen = resStar.length;
        // AT_RES: type(1)+len(1)+actual_bits(2)+resStar+padding to 4-byte boundary
        int resPadded = (resLen + 3) & ~3; // round up to multiple of 4
        int resAttrBytes = 2 + 2 + resPadded; // type+len(2) + actual_bits(2) + padded resStar
        // length field = resAttrBytes/4; verify it is a whole number
        int resAttrLen = resAttrBytes / 4; // AT_RES length in 4-byte words
        // Recalculate actual bytes from len field for consistency
        int resAttrTotalBytes = resAttrLen * 4;

        // AT_MAC: 20 bytes (type+len+reserved+MAC)
        int packetLen = 8 + resAttrTotalBytes + 20;
        byte[] packet = new byte[packetLen];

        int off = 0;
        // EAP header
        packet[off++] = CODE_RESPONSE;
        packet[off++] = identifier;
        packet[off++] = (byte) (packetLen >> 8);
        packet[off++] = (byte) (packetLen & 0xFF);
        packet[off++] = TYPE_EAP_AKA_PRIME;
        packet[off++] = SUBTYPE_AKA_CHALLENGE;
        packet[off++] = 0; // reserved
        packet[off++] = 0; // reserved

        // AT_RES
        packet[off++] = AT_RES;
        packet[off++] = (byte) resAttrLen;
        int actualBits = resLen * 8;
        packet[off++] = (byte) (actualBits >> 8);
        packet[off++] = (byte) (actualBits & 0xFF);
        System.arraycopy(resStar, 0, packet, off, resLen);
        off += resAttrTotalBytes - 4; // skip past resStar + padding (we started 4 bytes into attr)

        // AT_MAC (zeros for now)
        packet[off++] = AT_MAC;
        packet[off++] = 5; // 5 * 4 = 20 bytes
        // 2 reserved + 16 MAC zeros
        off += 18;

        // Compute and fill MAC
        byte[] mac = computeEapMac(kAutPrime, packet);
        System.arraycopy(mac, 0, packet, packetLen - 16, 16);

        return base64url(packet);
    }

    /**
     * Build a binary EAP-Response/AKA'-Reauthentication packet.
     *
     * @param identifier EAP identifier
     * @param kAutPrime  32-byte K_aut'
     * @param fastReauth if {@code true} includes AT_NONCE_S (signals fast reauthentication)
     */
    public static String buildReauthenticationResponse(byte identifier, byte[] kAutPrime, boolean fastReauth) {
        int nonceSAttrBytes = fastReauth ? 20 : 0; // AT_NONCE_S: type+len+reserved+16_nonce = 20
        int packetLen = 8 + nonceSAttrBytes + 20; // header + optional NONCE_S + AT_MAC
        byte[] packet = new byte[packetLen];

        int off = 0;
        packet[off++] = CODE_RESPONSE;
        packet[off++] = identifier;
        packet[off++] = (byte) (packetLen >> 8);
        packet[off++] = (byte) (packetLen & 0xFF);
        packet[off++] = TYPE_EAP_AKA_PRIME;
        packet[off++] = SUBTYPE_AKA_REAUTHENTICATION;
        packet[off++] = 0;
        packet[off++] = 0;

        if (fastReauth) {
            // AT_NONCE_S: type=0x15, len=5, reserved(2), nonce_s(16)
            packet[off++] = AT_NONCE_S;
            packet[off++] = 5;
            packet[off++] = 0; // reserved
            packet[off++] = 0; // reserved
            byte[] nonce = new byte[16];
            RANDOM.nextBytes(nonce);
            System.arraycopy(nonce, 0, packet, off, 16);
            off += 16;
        }

        // AT_MAC
        packet[off++] = AT_MAC;
        packet[off++] = 5;
        // 2 reserved + 16 zeros
        off += 18;

        byte[] mac = computeEapMac(kAutPrime, packet);
        System.arraycopy(mac, 0, packet, packetLen - 16, 16);

        return base64url(packet);
    }

    /**
     * Build a binary EAP-Response/AKA'-Synchronization-Failure packet.
     *
     * @param identifier EAP identifier
     * @param auts       14-byte AUTS value
     * @param kAutPrime  32-byte K_aut'
     */
    public static String buildSyncFailureResponse(byte identifier, byte[] auts, byte[] kAutPrime) {
        // AT_AUTS: type(1)+len(1)+auts(14) = 16 bytes; len = 4 (16/4)
        // AT_MAC: 20 bytes
        int packetLen = 8 + 16 + 20;
        byte[] packet = new byte[packetLen];

        int off = 0;
        packet[off++] = CODE_RESPONSE;
        packet[off++] = identifier;
        packet[off++] = (byte) (packetLen >> 8);
        packet[off++] = (byte) (packetLen & 0xFF);
        packet[off++] = TYPE_EAP_AKA_PRIME;
        packet[off++] = SUBTYPE_AKA_SYNC_FAILURE;
        packet[off++] = 0;
        packet[off++] = 0;

        // AT_AUTS: len=4 → 16 total bytes (type+len+14_bytes_auts)
        packet[off++] = AT_AUTS;
        packet[off++] = 4;
        int copyLen = Math.min(auts.length, AUTS_LENGTH);
        System.arraycopy(auts, 0, packet, off, copyLen);
        off += AUTS_LENGTH;

        // AT_MAC
        packet[off++] = AT_MAC;
        packet[off++] = 5;
        off += 18;

        byte[] mac = computeEapMac(kAutPrime, packet);
        System.arraycopy(mac, 0, packet, packetLen - 16, 16);

        return base64url(packet);
    }

    /**
     * Build a binary EAP-Failure packet and return its Base64url encoding.
     *
     * @param identifier EAP identifier (should match the last Request)
     */
    public static String buildFailure(byte identifier) {
        // EAP-Failure: Code(1) + ID(1) + Length(2) = 4 bytes, no Type
        byte[] packet = {CODE_FAILURE, identifier, 0, 4};
        return base64url(packet);
    }

    // -----------------------------------------------------------------------
    // Decoding
    // -----------------------------------------------------------------------

    /**
     * Decoded fields from an incoming EAP-Response packet.
     *
     * @param subtype     EAP-AKA' subtype byte
     * @param resStar     AT_RES value bytes (null if absent)
     * @param auts        AT_AUTS value bytes (null if absent)
     * @param hasFastReauth {@code true} if AT_NONCE_S was present (fast reauth)
     */
    public record DecodedResponse(
        byte   subtype,
        byte[] resStar,
        byte[] auts,
        boolean hasFastReauth
    ) {}

    /**
     * Decode a Base64url-encoded EAP-Response/AKA'-* packet.
     *
     * @param base64urlPayload Base64url (with or without padding) encoded EAP packet
     * @return decoded response, or {@code null} if the packet is invalid /
     *         not an EAP-Response/AKA' message
     */
    public static DecodedResponse decodeResponse(String base64urlPayload) {
        if (base64urlPayload == null || base64urlPayload.isBlank()) {
            return null;
        }
        try {
            byte[] packet = Base64.getUrlDecoder().decode(addPadding(base64urlPayload));
            if (packet.length < 8) return null;
            if (packet[0] != CODE_RESPONSE)        return null;
            if (packet[4] != TYPE_EAP_AKA_PRIME)   return null;

            byte subtype = packet[5];

            byte[] resStar = null;
            byte[] auts    = null;
            boolean hasNonceS = false;

            int off = 8;
            while (off + 2 <= packet.length) {
                int atType      = Byte.toUnsignedInt(packet[off]);
                int atLenWords  = Byte.toUnsignedInt(packet[off + 1]);
                int atTotalBytes = atLenWords * 4;
                if (atTotalBytes == 0 || off + atTotalBytes > packet.length) break;

                if (atType == Byte.toUnsignedInt(AT_RES) && atTotalBytes >= 4) {
                    int actualBits  = (Byte.toUnsignedInt(packet[off + 2]) << 8)
                                    |  Byte.toUnsignedInt(packet[off + 3]);
                    int resBytes = (actualBits + 7) / 8;
                    if (off + 4 + resBytes <= packet.length) {
                        resStar = Arrays.copyOfRange(packet, off + 4, off + 4 + resBytes);
                    }
                } else if (atType == Byte.toUnsignedInt(AT_AUTS) && atTotalBytes >= 2 + AUTS_LENGTH) {
                    auts = Arrays.copyOfRange(packet, off + 2, off + 2 + AUTS_LENGTH);
                } else if (atType == Byte.toUnsignedInt(AT_NONCE_S)) {
                    hasNonceS = true;
                }

                off += atTotalBytes;
            }

            boolean isFastReauth = (subtype == SUBTYPE_AKA_REAUTHENTICATION) && hasNonceS;
            return new DecodedResponse(subtype, resStar, auts, isFastReauth);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * Extract the identifier byte (byte[1]) from a Base64url-encoded EAP packet.
     * Returns {@code 0} if the packet is too short or cannot be decoded.
     */
    public static byte extractIdentifier(String base64urlPayload) {
        if (base64urlPayload == null || base64urlPayload.isBlank()) return 0;
        try {
            byte[] packet = Base64.getUrlDecoder().decode(addPadding(base64urlPayload));
            return packet.length >= 2 ? packet[1] : 0;
        } catch (Exception e) {
            return 0;
        }
    }

    /**
     * Verify AT_MAC in a binary EAP packet (provided as raw bytes with MAC already filled).
     * Returns {@code true} if the MAC is correct, or if no AT_MAC attribute is present
     * (lenient mode used for reauthentication / sync-failure responses).
     */
    public static boolean verifyMac(byte[] kAutPrime, byte[] packet) {
        int macOffset = findMacValueOffset(packet);
        if (macOffset < 0) {
            // No AT_MAC — accept leniently
            return true;
        }
        byte[] copy = Arrays.copyOf(packet, packet.length);
        Arrays.fill(copy, macOffset, macOffset + 16, (byte) 0);
        byte[] expected = computeEapMac(kAutPrime, copy);
        byte[] actual   = Arrays.copyOfRange(packet, macOffset, macOffset + 16);
        return Arrays.equals(expected, actual);
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    private static byte[] assembleChallengePacket(
        byte code, byte identifier,
        byte[] rand, byte[] autn,
        String servingNetworkName, byte[] kAutPrime
    ) {
        byte[] snnBytes  = servingNetworkName.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        int snnLen       = snnBytes.length;
        int snnPadded    = (snnLen + 3) & ~3; // round up to 4-byte boundary
        // AT_KDF_INPUT: type(1)+len(1)+actual_len(2)+snnBytes+padding
        int snnAttrBytes = 2 + 2 + snnPadded;
        int snnAttrWords = snnAttrBytes / 4;

        // Total packet:
        // 8 (header) + 20 (AT_RAND) + 20 (AT_AUTN) + 4 (AT_KDF) + snnAttrBytes (AT_KDF_INPUT) + 20 (AT_MAC)
        int packetLen = 8 + 20 + 20 + 4 + snnAttrBytes + 20;
        byte[] packet = new byte[packetLen];

        int off = 0;
        // EAP header
        packet[off++] = code;
        packet[off++] = identifier;
        packet[off++] = (byte) (packetLen >> 8);
        packet[off++] = (byte) (packetLen & 0xFF);
        packet[off++] = TYPE_EAP_AKA_PRIME;
        packet[off++] = SUBTYPE_AKA_CHALLENGE;
        packet[off++] = 0; // reserved
        packet[off++] = 0; // reserved

        // AT_RAND: type=1, len=5 (20 bytes), reserved(2)+RAND(16)
        packet[off++] = AT_RAND;
        packet[off++] = 5;
        packet[off++] = 0; // reserved
        packet[off++] = 0;
        System.arraycopy(rand, 0, packet, off, 16);
        off += 16;

        // AT_AUTN: type=2, len=5, reserved(2)+AUTN(16)
        packet[off++] = AT_AUTN;
        packet[off++] = 5;
        packet[off++] = 0;
        packet[off++] = 0;
        System.arraycopy(autn, 0, packet, off, 16);
        off += 16;

        // AT_KDF: type=0x18, len=1 (4 bytes), value=0x0001
        packet[off++] = AT_KDF;
        packet[off++] = 1;
        packet[off++] = 0;
        packet[off++] = KDF_PRF_HMAC_SHA256;

        // AT_KDF_INPUT: type=0x17, len=snnAttrWords, actual_len(2)+snnBytes+padding
        packet[off++] = AT_KDF_INPUT;
        packet[off++] = (byte) snnAttrWords;
        packet[off++] = (byte) (snnLen >> 8);
        packet[off++] = (byte) (snnLen & 0xFF);
        System.arraycopy(snnBytes, 0, packet, off, snnLen);
        off += snnPadded; // skip snnLen bytes + padding (rest already zero)

        // AT_MAC: type=0x0B, len=5, reserved(2)+MAC(16) — zeros for MAC computation
        packet[off++] = AT_MAC;
        packet[off++] = 5;
        // 2 reserved + 16 zeros — already zero from array init
        // off += 18; // final attribute, no need to advance

        // Compute and fill MAC
        byte[] mac = computeEapMac(kAutPrime, packet);
        System.arraycopy(mac, 0, packet, packetLen - 16, 16);

        return packet;
    }

    /** HMAC-SHA-256(K_aut', packet), taking the first 16 bytes. */
    static byte[] computeEapMac(byte[] kAutPrime, byte[] packet) {
        try {
            Mac hmac = Mac.getInstance("HmacSHA256");
            hmac.init(new SecretKeySpec(kAutPrime, "HmacSHA256"));
            byte[] full = hmac.doFinal(packet);
            return Arrays.copyOf(full, 16);
        } catch (Exception e) {
            throw new IllegalStateException("EAP AT_MAC computation failed", e);
        }
    }

    /** Returns the byte offset of the 16-byte MAC value inside AT_MAC, or -1 if not found. */
    private static int findMacValueOffset(byte[] packet) {
        if (packet.length < 8) return -1;
        int off = 8;
        while (off + 2 <= packet.length) {
            int atType     = Byte.toUnsignedInt(packet[off]);
            int atLenWords = Byte.toUnsignedInt(packet[off + 1]);
            int atBytes    = atLenWords * 4;
            if (atBytes == 0 || off + atBytes > packet.length) break;
            if (atType == Byte.toUnsignedInt(AT_MAC) && atBytes >= 20) {
                return off + 4; // MAC value starts after type(1)+len(1)+reserved(2)
            }
            off += atBytes;
        }
        return -1;
    }

    private static byte randomByte() {
        byte[] b = new byte[1];
        RANDOM.nextBytes(b);
        return b[0];
    }

    private static String base64url(byte[] data) {
        return Base64.getUrlEncoder().withoutPadding().encodeToString(data);
    }

    private static String addPadding(String s) {
        int rem = s.length() % 4;
        return rem == 0 ? s : s + "=".repeat(4 - rem);
    }
}
