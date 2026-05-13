package com.ausf.controlplane.api;

import java.util.Map;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/nnrf-disc/v1/notification")
class NfDiscoveryNotifyController {

    private static final Logger log = LoggerFactory.getLogger(NfDiscoveryNotifyController.class);

    @PostMapping
    ResponseEntity<Void> receiveNotification(@RequestBody Map<String, Object> notificationData) {
        String event = String.valueOf(notificationData.get("event"));
        Object nfProfile = notificationData.get("nfProfile");
        log.info("NRF discovery notification: event={} nfProfile={}", event, nfProfile);
        return ResponseEntity.noContent().build();
    }
}
