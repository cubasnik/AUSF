-- V1: initial schema for AUSF subscriber profiles.
-- Matches JPA entity com.ausf.controlplane.subscriber.SubscriberProfile
-- with @Table(name = "subscribers") and Spring Boot default camelCase→snake_case column naming.
-- aka_algorithm is nullable: it may be absent until the auth method is resolved.

CREATE TABLE IF NOT EXISTS subscribers (
    supi                 VARCHAR(255) NOT NULL,
    auth_method          VARCHAR(50)  NOT NULL,
    aka_algorithm        VARCHAR(50),
    permanent_key        VARCHAR(255) NOT NULL,
    opc                  VARCHAR(255) NOT NULL,
    serving_network_name VARCHAR(255) NOT NULL,
    sequence_number      BIGINT       NOT NULL DEFAULT 0,
    routing_indicator    VARCHAR(255) NOT NULL,
    CONSTRAINT pk_subscribers PRIMARY KEY (supi)
);
