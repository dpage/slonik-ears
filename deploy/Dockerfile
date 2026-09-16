# Build and package the Slonik Ears relay server.
#
# Only the server is containerised: the listener needs a microphone and lives
# on a Mac in the room.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
ARG COMMIT=""
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/dpage/slonik-ears/internal/version.Version=${VERSION} -X github.com/dpage/slonik-ears/internal/version.Commit=${COMMIT}" \
    -o /out/ears-server ./cmd/ears-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 ears && \
    mkdir -p /data && chown ears:ears /data
COPY --from=build /out/ears-server /usr/local/bin/ears-server
USER ears
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
ENV EARS_ADDR=":8080" EARS_DATA_DIR="/data"
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["ears-server"]
