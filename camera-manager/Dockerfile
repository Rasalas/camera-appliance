FROM node:22-bookworm AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-bookworm AS backend
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

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates iproute2 docker.io libsecret-tools openssh-client && rm -rf /var/lib/apt/lists/*
COPY --from=backend /out/camera-appliance /usr/local/bin/camera-appliance
COPY --from=frontend /src/frontend/dist /opt/camera-appliance/frontend/dist
COPY compose.yaml /opt/camera-appliance/compose.yaml
WORKDIR /opt/camera-appliance
EXPOSE 8091
ENTRYPOINT ["/usr/local/bin/camera-appliance"]
CMD ["serve"]
