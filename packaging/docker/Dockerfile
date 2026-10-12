# Build-only, digest-pinned official images. No registry push or public release.
FROM node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402 AS web
WORKDIR /src/cmd/ndg-desktop/frontend
COPY cmd/ndg-desktop/frontend/package*.json ./
RUN npm ci
COPY cmd/ndg-desktop/frontend/ ./
RUN npx tsc && npx vite build --config vite.web.config.ts && cp /src/internal/apihttp/webdist/web.html /src/internal/apihttp/webdist/index.html
FROM golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ndg-server ./cmd/ndg-server
COPY internal ./internal
COPY --from=web /src/internal/apihttp/webdist ./internal/apihttp/webdist
ARG TARGETOS=linux
ARG TARGETARCH
ARG SOURCE_SHA=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X github.com/FNB2026/nas-data-governance/internal/version.Commit=$SOURCE_SHA -X github.com/FNB2026/nas-data-governance/internal/version.Channel=web-experimental" -o /out/ndg-server ./cmd/ndg-server
FROM scratch
ARG SOURCE_SHA=unknown
LABEL org.opencontainers.image.source="https://github.com/FNB2026/nas-data-governance" org.opencontainers.image.revision=$SOURCE_SHA org.opencontainers.image.licenses="Apache-2.0" org.opencontainers.image.title="NDG read-only Web experiment"
COPY --from=build /out/ndg-server /ndg-server
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/ndg-server","-config","/config/server.json"]
