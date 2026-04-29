package com.ausf.controlplane.api;

import com.ausf.controlplane.authentication.AuthenticationManager;
import com.ausf.controlplane.authentication.AuthenticationRequest;
import com.ausf.controlplane.authentication.AuthenticationResponse;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/control-plane/v1/auth")
public class AuthenticationController {
    private final AuthenticationManager authenticationManager;

    public AuthenticationController(AuthenticationManager authenticationManager) {
        this.authenticationManager = authenticationManager;
    }

    @PostMapping("/initiate")
    public ResponseEntity<AuthenticationResponse> initiate(@RequestBody AuthenticationRequest request) {
        AuthenticationResponse response = authenticationManager.initiateAuthentication(
            request.getSupi(),
            request.getServingNetworkName(),
            request.getAuthType()
        );
        if (!response.getSuccess()) {
            return ResponseEntity.status(resolveFailureStatusCode(response)).body(response);
        }
        return ResponseEntity.status(HttpStatus.CREATED).body(response);
    }

    @PostMapping("/{supi}/confirm")
    public ResponseEntity<AuthenticationResponse> confirm(
        @PathVariable("supi") String supi,
        @RequestBody AuthenticationRequest request
    ) {
        AuthenticationResponse response = authenticationManager.verifyAuthenticationResponse(
            supi,
            request.getResStar(),
            request.getEapPayload()
        );
        if (!response.getSuccess()) {
            return ResponseEntity.status(resolveFailureStatusCode(response)).body(response);
        }
        return ResponseEntity.ok(response);
    }

    @GetMapping("/{supi}")
    public ResponseEntity<Object> context(@PathVariable("supi") String supi) {
        Object context = authenticationManager.getContext(supi);
        if (context == null) {
            return ResponseEntity.notFound().build();
        }
        return ResponseEntity.ok(context);
    }

    private int resolveFailureStatusCode(AuthenticationResponse response) {
        return switch (response.getErrorCode()) {
            case "SUBSCRIBER_NOT_FOUND", "CONTEXT_NOT_FOUND" -> HttpStatus.NOT_FOUND.value();
            case "AUTHENTICATION_REJECTED" -> HttpStatus.UNAUTHORIZED.value();
            case "CONTROL_PLANE_UNAVAILABLE" -> HttpStatus.BAD_GATEWAY.value();
            default -> HttpStatus.INTERNAL_SERVER_ERROR.value();
        };
    }
}
