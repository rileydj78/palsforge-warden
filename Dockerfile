# Palsforge Warden as a sidecar next to a Palworld server container.
# Build: docker build -t palsforge-warden .
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /warden ./cmd/warden

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 1000 warden && mkdir /data && chown warden /data
COPY --from=build /warden /usr/local/bin/warden
USER warden
VOLUME ["/data"]
# The panel listens on 127.0.0.1 inside the container on purpose: publish it only through a
# Tailscale sidecar (see docs/docker.md), never with -p to the internet.
ENTRYPOINT ["warden", "-data", "/data"]
CMD ["-server", "/palworld"]
