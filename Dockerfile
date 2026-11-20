# Build a static nogui binary, then keep only that binary plus a data dir.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -tags nogui -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" -o /out/shellkit .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 shellkit && mkdir -p /data && chown shellkit /data
COPY --from=build /out/shellkit /usr/local/bin/shellkit
USER shellkit
ENV SHELLKIT_HOME=/data \
    SHELLKIT_ADDR=0.0.0.0:3427
VOLUME /data
EXPOSE 3427
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- "http://127.0.0.1:3427/healthz" >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/shellkit"]
CMD ["serve"]
