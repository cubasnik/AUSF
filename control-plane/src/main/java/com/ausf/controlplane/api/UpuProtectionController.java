package com.ausf.controlplane.api;

import com.ausf.controlplane.authentication.AuthenticationManager;
import com.ausf.controlplane.authentication.AuthenticationManager.ProtectionResult;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/**
 * Internal control-plane endpoint for Nausf_UPUProtection (TS 29.509 §6.3).
 *
 * <p>Called by the Go microservice after a successful UE authentication.
 * Computes MAC-UPU using KAUSF of the authenticated context.
 *
 * <p>POST /control-plane/v1/auth/{authCtxId}/upu-protection
 */
@RestController
@RequestMapping("/control-plane/v1/auth")
public class UpuProtectionController {

    private final AuthenticationManager authenticationManager;

    public UpuProtectionController(AuthenticationManager authenticationManager) {
        this.authenticationManager = authenticationManager;
    }

    @PostMapping("/{authCtxId}/upu-protection")
    public ResponseEntity<UpuProtectionResponse> computeUPUProtection(
        @PathVariable("authCtxId") String authCtxId,
        @RequestBody UpuProtectionRequest request
    ) {
        ProtectionResult result = authenticationManager.computeUPUProtection(
            authCtxId,
            request.getUpuData(),
            request.isAckIndication(),
            request.getUpuHeader()
        );
        return ResponseEntity.ok(new UpuProtectionResponse(result.mac(), result.counter()));
    }

    public static class UpuProtectionRequest {
        private String upuData;
        private boolean ackIndication;
        // Optional TS 29.509 OAS3 schema fields (Table 6.3.6.2.2-1).
        private String upuHeader;
        private Boolean provisioning3gppInd;

        public String getUpuData() { return upuData; }
        public void setUpuData(String upuData) { this.upuData = upuData; }
        public boolean isAckIndication() { return ackIndication; }
        public void setAckIndication(boolean ackIndication) { this.ackIndication = ackIndication; }
        public String getUpuHeader() { return upuHeader; }
        public void setUpuHeader(String upuHeader) { this.upuHeader = upuHeader; }
        public Boolean getProvisioning3gppInd() { return provisioning3gppInd; }
        public void setProvisioning3gppInd(Boolean provisioning3gppInd) { this.provisioning3gppInd = provisioning3gppInd; }
    }

    public record UpuProtectionResponse(String upuMacIausf, String counterUpu) {}
}
