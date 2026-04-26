FROM mcr.microsoft.com/devcontainers/java:25-jdk-bookworm AS builder

WORKDIR /app

RUN rm -f /etc/apt/sources.list.d/yarn.list /etc/apt/sources.list.d/yarn.sources \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends maven \
	&& rm -rf /var/lib/apt/lists/*

COPY pom.xml ./
COPY src ./src

RUN mvn -q -DskipTests package

FROM mcr.microsoft.com/devcontainers/java:25-jdk-bookworm

WORKDIR /app

COPY --from=builder /app/target/control-plane-1.0.0.jar /app/control-plane.jar

EXPOSE 8081

ENTRYPOINT ["java", "-jar", "/app/control-plane.jar"]