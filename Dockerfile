# syntax=docker/dockerfile:1
# Build: docker build -t bcm .      Run: see docs/architecture.md, "Deploying".
# Base images are pinned by digest (refresh: docker pull <tag>, copy the RepoDigest; Dependabot-style bumps are a deliberate edit) and
# Tailwind by version, so the same checkout builds the same image; -trimpath, -buildvcs=false and an empty build id keep the binary free of paths and clock.

# 1. The stylesheet (app.css is generated and not committed; the server embeds it).
FROM alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8 AS css
ARG TARGETARCH
ARG TAILWIND_VERSION=v4.3.3
RUN apk add --no-cache curl libstdc++ libgcc
WORKDIR /src
RUN arch=$([ "$TARGETARCH" = "arm64" ] && echo arm64 || echo x64) && \
    url=https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-${arch}-musl && \
    [ "$TAILWIND_VERSION" = "latest" ] || url=https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${arch}-musl && \
    curl -fsSL "$url" -o /usr/local/bin/tailwindcss && chmod +x /usr/local/bin/tailwindcss
COPY tailwind.css ./
COPY internal/web ./internal/web
RUN tailwindcss build -i tailwind.css -o /app.css --minified

# 2. The binary: static, no cgo; time/tzdata and the rules definitions are embedded or read from /app/definitions.
FROM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=css /app.css internal/web/static/app.css
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o /server ./cmd/server

# 3. Runtime: distroless static (CA certificates, tzdata, no shell, no package manager), non-root numeric uid, nothing but the binary
# and the shipped rules files. It writes nothing to disk, so the root filesystem can be read-only (compose.yaml does that).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=build /server /app/server
COPY --from=build /src/definitions /app/definitions
ENV PORT=8080 APP_ENV=production
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=15s --timeout=4s --start-period=20s --retries=3 CMD ["/app/server", "healthcheck"]
ENTRYPOINT ["/app/server"]
