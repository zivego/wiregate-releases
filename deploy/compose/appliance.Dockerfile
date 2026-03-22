## ---------------------------------------------------------------------------
## appliance.Dockerfile — WireGate all-in-one: backend + WireGuard server.
##
## Includes wireguard-tools, iproute2, iptables so the container can create
## and manage the wg0 interface directly. Keys are generated on first boot.
##
## Requires: --cap-add NET_ADMIN --device /dev/net/tun
## ---------------------------------------------------------------------------

FROM golang:1.24-alpine AS build
ARG WIREGATE_VERSION=dev
ARG WIREGATE_COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY openapi/ ./openapi/
COPY pkg/ ./pkg/
RUN go build -ldflags "-X github.com/zivego/wiregate/internal/version.Version=${WIREGATE_VERSION} -X github.com/zivego/wiregate/internal/version.CommitSHA=${WIREGATE_COMMIT} -X github.com/zivego/wiregate/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /out/wiregate-server ./cmd/wiregate-server

FROM alpine:3.20
RUN apk add --no-cache wireguard-tools iproute2 iptables docker-cli docker-cli-compose
RUN mkdir -p /var/lib/wiregate /etc/wireguard
COPY --from=build /out/wiregate-server /usr/local/bin/wiregate-server
COPY deploy/compose/wg-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 8080 55182/udp
ENTRYPOINT ["/entrypoint.sh"]
