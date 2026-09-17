# Build and package the Slonik Ears relay server.
#
# Only the server is containerised: the listener needs a microphone and a
# Whisper backend, and lives on a machine in the room.
#
# The Go stage runs on the BUILD platform and cross-compiles to the TARGET
# one. The server has no cgo, so a multi-architecture image needs no emulation
# and builds in about the time a single-architecture one does.

FROM --platform=$BUILDPLATFORM node:26-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=""
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X github.com/dpage/slonik-ears/internal/version.Version=${VERSION} -X github.com/dpage/slonik-ears/internal/version.Commit=${COMMIT}" \
    -o /out/ears-server ./cmd/ears-server

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 10001 ears && \
    mkdir -p /data && chown ears:ears /data
COPY --from=build /out/ears-server /usr/local/bin/ears-server
USER ears
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
ENV EARS_ADDR=":8080" \
    EARS_DATA_DIR="/data"
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

# Labels are what makes a package page on GHCR link back to its source.
LABEL org.opencontainers.image.title="Slonik Ears" \
      org.opencontainers.image.description="Live event transcription relay and attendee web app" \
      org.opencontainers.image.source="https://github.com/dpage/slonik-ears" \
      org.opencontainers.image.licenses="PostgreSQL"

ENTRYPOINT ["ears-server"]
