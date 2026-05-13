package com.ausf.controlplane.api;

import java.util.Map;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/**
 * Receives NF-status change notifications from the NRF
 * (Nnrf_NFManagement subscription callbacks, TS 29.510 §5.2.2.4).
 *
 * <p>The NRF POSTs a {@code NotificationData} body to the URI registered via
 * {@code POST /nnrf-nfm/v1/subscriptions}. This controller acknowledges the
 * notification with 204 No Content and logs the event.
 */
@RestController
@RequestMapping("/nnrf-nfm/v1/notification")
class NfStatusNotifyController {

    private static final Logger log = LoggerFactory.getLogger(NfStatusNotifyController.class);

    /**
     * Handles an incoming NF-status notification from the NRF.
     *
     * <p>Per TS 29.510 Table 5.3.2.3-1, the body is a {@code NotificationData}
     * object containing {@code event} and {@code nfInstanceUri} at minimum.
     */
    @PostMapping
    ResponseEntity<Void> receiveNotification(@RequestBody Map<String, Object> notificationData) {
        String event = String.valueOf(notificationData.getOrDefault("event", "UNKNOWN"));
        String nfInstanceUri = String.valueOf(notificationData.getOrDefault("nfInstanceUri", ""));
        log.info("NRF NF-status notification: event={} nfInstanceUri={}", event, nfInstanceUri);
        return ResponseEntity.noContent().build();
    }
}
