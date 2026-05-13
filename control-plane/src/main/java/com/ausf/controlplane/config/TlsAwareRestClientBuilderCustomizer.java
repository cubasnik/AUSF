package com.ausf.controlplane.config;

import java.io.IOException;
import java.net.http.HttpClient;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.GeneralSecurityException;
import java.security.KeyStore;
import java.security.cert.Certificate;
import java.security.cert.CertificateFactory;
import java.time.Duration;
import java.util.Collection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManagerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.client.JdkClientHttpRequestFactory;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

@Component
public class TlsAwareRestClientBuilderCustomizer {

    private static final Duration CONNECT_TIMEOUT = Duration.ofSeconds(5);
    private static final Duration READ_TIMEOUT = Duration.ofSeconds(10);

    private final String caCertFile;

    public TlsAwareRestClientBuilderCustomizer(@Value("${ausf.tls.client.ca-cert-file:}") String caCertFile) {
        this.caCertFile = caCertFile == null ? "" : caCertFile.trim();
    }

    public RestClient.Builder customize(RestClient.Builder builder) {
        if (caCertFile.isBlank()) {
            HttpClient httpClient = HttpClient.newBuilder()
                .connectTimeout(CONNECT_TIMEOUT)
                .build();
            JdkClientHttpRequestFactory factory = new JdkClientHttpRequestFactory(httpClient);
            factory.setReadTimeout(READ_TIMEOUT);
            return builder.requestFactory(factory);
        }

        try {
            HttpClient httpClient = HttpClient.newBuilder()
                .connectTimeout(CONNECT_TIMEOUT)
                .sslContext(buildSslContext(Path.of(caCertFile)))
                .build();
            JdkClientHttpRequestFactory factory = new JdkClientHttpRequestFactory(httpClient);
            factory.setReadTimeout(READ_TIMEOUT);
            return builder.requestFactory(factory);
        } catch (IOException | GeneralSecurityException exception) {
            throw new IllegalStateException("Failed to configure TLS trust for outbound RestClient", exception);
        }
    }

    public static TlsAwareRestClientBuilderCustomizer noop() {
        return new TlsAwareRestClientBuilderCustomizer("");
    }

    private SSLContext buildSslContext(Path certificatePath) throws IOException, GeneralSecurityException {
        byte[] pemBytes = Files.readAllBytes(certificatePath);
        CertificateFactory certificateFactory = CertificateFactory.getInstance("X.509");
        Collection<? extends Certificate> certificates = certificateFactory.generateCertificates(
            new java.io.ByteArrayInputStream(pemBytes)
        );
        if (certificates.isEmpty()) {
            throw new IllegalStateException("No X.509 certificates found in " + certificatePath);
        }

        KeyStore trustStore = KeyStore.getInstance(KeyStore.getDefaultType());
        trustStore.load(null, null);

        int index = 0;
        for (Certificate certificate : certificates) {
            trustStore.setCertificateEntry("ausf-ca-" + index++, certificate);
        }

        TrustManagerFactory trustManagerFactory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        trustManagerFactory.init(trustStore);

        SSLContext sslContext = SSLContext.getInstance("TLS");
        sslContext.init(null, trustManagerFactory.getTrustManagers(), null);
        return sslContext;
    }
}