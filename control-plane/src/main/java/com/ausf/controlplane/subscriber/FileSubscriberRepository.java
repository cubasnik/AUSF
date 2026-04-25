package com.ausf.controlplane.subscriber;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import jakarta.annotation.PostConstruct;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.core.io.Resource;
import org.springframework.stereotype.Repository;

@Repository
public class FileSubscriberRepository implements SubscriberRepository {
    private final ObjectMapper objectMapper;
    private final Resource seedResource;
    private final Path storagePath;
    private final Map<String, SubscriberProfile> subscribers = new ConcurrentHashMap<>();

    public FileSubscriberRepository(
        ObjectMapper objectMapper,
        @Value("classpath:seed/subscribers.json") Resource seedResource,
        @Value("${ausf.subscriber.store:./data/subscribers.json}") String storagePath
    ) {
        this.objectMapper = objectMapper;
        this.seedResource = seedResource;
        this.storagePath = Path.of(storagePath);
    }

    @PostConstruct
    void initialize() throws IOException {
        if (Files.notExists(storagePath)) {
            Files.createDirectories(storagePath.getParent());
            try (InputStream inputStream = seedResource.getInputStream()) {
                Files.copy(inputStream, storagePath);
            }
        }

        try (InputStream inputStream = Files.newInputStream(storagePath)) {
            List<SubscriberProfile> profiles = objectMapper.readValue(inputStream, new TypeReference<List<SubscriberProfile>>() {});
            for (SubscriberProfile profile : profiles) {
                subscribers.put(profile.getSupi(), profile);
            }
        }
    }

    @Override
    public Optional<SubscriberProfile> findBySupi(String supi) {
        return Optional.ofNullable(subscribers.get(supi));
    }

    public Map<String, SubscriberProfile> snapshot() {
        return Collections.unmodifiableMap(subscribers);
    }
}
