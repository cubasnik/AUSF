FROM golang:1.26-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY vendor ./vendor

COPY cmd ./cmd
COPY internal ./internal

RUN go build -mod=vendor -o /out/ausf ./cmd/ausf

FROM golang:1.26-bookworm

WORKDIR /app

COPY --from=builder /out/ausf /usr/local/bin/ausf

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/ausf"]