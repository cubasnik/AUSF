FROM mcr.microsoft.com/devcontainers/go:1-1.22-bookworm AS builder

WORKDIR /app

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /out/ausf ./cmd/ausf

FROM mcr.microsoft.com/devcontainers/go:1-1.22-bookworm

WORKDIR /app

COPY --from=builder /out/ausf /usr/local/bin/ausf

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/ausf"]