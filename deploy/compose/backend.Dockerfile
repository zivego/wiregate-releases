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
RUN adduser -D -h /app wiregate
RUN mkdir -p /var/lib/wiregate && chown -R wiregate:wiregate /var/lib/wiregate
USER wiregate
WORKDIR /app
COPY --from=build /out/wiregate-server /app/wiregate-server
EXPOSE 8080
ENTRYPOINT ["/app/wiregate-server"]
