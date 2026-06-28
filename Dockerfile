# Build stage: compile a static binary.
FROM golang:1.25-alpine AS build

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build (templ output is committed, so no codegen step is needed here).
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o /out/tdrive .

# Runtime stage: minimal image, non-root, persistent data volume.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 tdrive && \
    mkdir -p /data && chown tdrive:tdrive /data

COPY --from=build /out/tdrive /usr/local/bin/tdrive

USER tdrive
WORKDIR /data
VOLUME ["/data"]

EXPOSE 3000

# All configuration is via CLI flags; override CMD to change port, enable FTP, etc.
ENTRYPOINT ["tdrive"]
CMD ["/data"]
