FROM node:22-bookworm AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
COPY LICENSE /src/LICENSE
COPY camera-manager/internal/snapshotupload/stamp-font.json /src/camera-manager/internal/snapshotupload/stamp-font.json
RUN npm run build

FROM golang:1.26-bookworm AS backend
ARG VERSION=dev
ARG COMMIT=local
ARG BUILD_TIME=
WORKDIR /src
COPY camera-manager/go.mod camera-manager/go.sum* ./camera-manager/
WORKDIR /src/camera-manager
RUN go mod download
WORKDIR /src
COPY camera-manager ./camera-manager
COPY --from=frontend /src/frontend/dist ./frontend/dist
WORKDIR /src/camera-manager
RUN CGO_ENABLED=0 go build \
  -ldflags "-X camera-appliance/camera-manager/internal/version.Version=$VERSION -X camera-appliance/camera-manager/internal/version.Commit=$COMMIT -X camera-appliance/camera-manager/internal/version.BuildTime=$BUILD_TIME" \
  -o /out/camera-appliance ./cmd/camera-appliance

FROM docker:29.8.2-cli@sha256:b1805116a6a86cc591b5d5f60a910a0715cdcc9d18d866ad68b1457ead25c35c AS docker-cli

FROM debian:bookworm-slim AS runtime
# Bookworm's Docker 20.10 only supports API 1.41. Modern host daemons may
# require >= 1.44, so ship a pinned CLI plus Compose/Buildx from Docker.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates iproute2 libsecret-tools openssh-client ffmpeg && rm -rf /var/lib/apt/lists/*
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker-cli /usr/local/libexec/docker/cli-plugins/ /usr/local/libexec/docker/cli-plugins/
# Older stack helpers invoke docker-compose as their entrypoint.
RUN ln -s /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/bin/docker-compose \
  && docker --version && docker compose version && docker buildx version

FROM runtime AS release
COPY --from=backend /out/camera-appliance /usr/local/bin/camera-appliance
COPY --from=frontend /src/frontend/dist /opt/camera-appliance/frontend/dist
COPY compose.yaml /opt/camera-appliance/compose.yaml
WORKDIR /opt/camera-appliance
EXPOSE 8091
ENTRYPOINT ["/usr/local/bin/camera-appliance"]
CMD ["serve"]
