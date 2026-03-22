FROM golang:1.24-alpine AS build
ARG WIREGATE_VERSION=dev
ARG WIREGATE_COMMIT=unknown
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY openapi ./openapi
COPY pkg ./pkg
RUN go build -ldflags "-X github.com/zivego/wiregate/internal/version.Version=${WIREGATE_VERSION} -X github.com/zivego/wiregate/internal/version.CommitSHA=${WIREGATE_COMMIT} -X github.com/zivego/wiregate/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /out/wiregate-server ./cmd/wiregate-server

FROM alpine:3.20
RUN apk add --no-cache su-exec
RUN adduser -D -h /app wiregate
RUN mkdir -p /var/lib/wiregate && chown -R wiregate:wiregate /var/lib/wiregate
WORKDIR /app
COPY --from=build /out/wiregate-server /app/wiregate-server
COPY deploy/compose/backend-entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]
