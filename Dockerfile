# Stage 1: Build the Go tracker binary
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Cache module deps separately from sources
COPY go-tracker/go.mod ./go-tracker/
RUN cd go-tracker && go mod download

COPY go-tracker/ ./go-tracker/

# Static binary, stripped, no cgo (so we can run on scratch/distroless if
# desired and don't need libc in the runtime image).
RUN cd go-tracker && \
    CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/tracker \
        ./cmd/tracker

# Stage 2: Runtime image
FROM alpine:latest

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1000 trackeruser

WORKDIR /app
COPY --from=builder /out/tracker /app/tracker
RUN chown -R trackeruser:trackeruser /app

USER trackeruser

# 50000/udp - INFO_PACKET, WBKA, hole-punch
# 50000/tcp - full game list
# 50001/tcp - interesting games subset
# 50005/tcp - HTTP web view
EXPOSE 50000/tcp
EXPOSE 50000/udp
EXPOSE 50001/tcp
EXPOSE 50005/tcp

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 50000 || exit 1

ENTRYPOINT ["/app/tracker"]
