FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o hermes ./cmd/server

# --- Runtime ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates sqlite docker-cli tzdata

WORKDIR /app
COPY --from=builder /build/hermes /app/hermes
COPY config.yaml /app/config.yaml

RUN mkdir -p /data/logs

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/app/hermes"]
CMD ["-config", "/app/config.yaml"]
