FROM mcr.microsoft.com/devcontainers/java:25-jdk-bookworm AS java-test-base

WORKDIR /app

RUN rm -f /etc/apt/sources.list.d/yarn.list /etc/apt/sources.list.d/yarn.sources \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends maven \
	&& rm -rf /var/lib/apt/lists/*

COPY pom.xml ./

RUN --mount=type=cache,target=/root/.m2 \
	mvn -q -B -DskipTests dependency:go-offline

FROM java-test-base AS builder

COPY src ./src

RUN --mount=type=cache,target=/root/.m2 \
	mvn -q -DskipTests package

FROM mcr.microsoft.com/devcontainers/java:25-jdk-bookworm AS runtime-from-source

WORKDIR /app

COPY --from=builder /app/target/control-plane-1.0.0.jar /app/control-plane.jar

EXPOSE 8081

ENTRYPOINT ["java", "-jar", "/app/control-plane.jar"]

FROM mcr.microsoft.com/devcontainers/java:25-jdk-bookworm AS runtime-from-host-jar

WORKDIR /app

COPY target/control-plane-1.0.0.jar /app/control-plane.jar

EXPOSE 8081

ENTRYPOINT ["java", "-jar", "/app/control-plane.jar"]