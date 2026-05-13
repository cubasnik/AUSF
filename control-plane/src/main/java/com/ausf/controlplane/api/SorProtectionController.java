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
 * Internal control-plane endpoint for Nausf_SoRProtection (TS 29.509 §6.2).
 *
 * <p>Called by the Go microservice after a successful UE authentication.
 * Computes MAC-SoR using KAUSF of the authenticated context.
 *
 * <p>POST /control-plane/v1/auth/{authCtxId}/sor-protection
 */
@RestController
@RequestMapping("/control-plane/v1/auth")
public class SorProtectionController {

    private final AuthenticationManager authenticationManager;

    public SorProtectionController(AuthenticationManager authenticationManager) {
        this.authenticationManager = authenticationManager;
    }

    @PostMapping("/{authCtxId}/sor-protection")
    public ResponseEntity<SorProtectionResponse> computeSoRProtection(
        @PathVariable("authCtxId") String authCtxId,
        @RequestBody SorProtectionRequest request
    ) {
        ProtectionResult result = authenticationManager.computeSoRProtection(
            authCtxId,
            request.getSteeringContainer(),
            request.isAckIndication(),
            request.getSorHeader()
        );
        return ResponseEntity.ok(new SorProtectionResponse(result.mac(), result.counter(), request.getStorageIndicator()));
    }

    public static class SorProtectionRequest {
        private String steeringContainer;
        private boolean ackIndication;
        // Optional TS 29.509 OAS3 schema fields (Table 6.2.6.2.2-1).
        private String sorHeader;
        private String storageIndicator;
        private Boolean provisioning3gppInd;

        public String getSteeringContainer() { return steeringContainer; }
        public void setSteeringContainer(String steeringContainer) { this.steeringContainer = steeringContainer; }
        public boolean isAckIndication() { return ackIndication; }
        public void setAckIndication(boolean ackIndication) { this.ackIndication = ackIndication; }
        public String getSorHeader() { return sorHeader; }
        public void setSorHeader(String sorHeader) { this.sorHeader = sorHeader; }
        public String getStorageIndicator() { return storageIndicator; }
        public void setStorageIndicator(String storageIndicator) { this.storageIndicator = storageIndicator; }
        public Boolean getProvisioning3gppInd() { return provisioning3gppInd; }
        public void setProvisioning3gppInd(Boolean provisioning3gppInd) { this.provisioning3gppInd = provisioning3gppInd; }
    }

    public record SorProtectionResponse(String sorMacIausf, String counterSor, String storageIndicator) {}
}
