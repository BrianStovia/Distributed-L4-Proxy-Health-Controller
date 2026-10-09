# Stage 1: Build Go Control Plane
FROM golang:1.23-bookworm AS builder

WORKDIR /workspace

COPY controlplane/go.mod controlplane/go.sum* /workspace/controlplane/
WORKDIR /workspace/controlplane
RUN go mod download

COPY proto/ /workspace/proto/
COPY controlplane/ /workspace/controlplane/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/controlplane ./cmd/server

# Stage 2: Distroless minimal runtime container
FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/controlplane /usr/local/bin/controlplane

ENV GRPC_PORT=50051 \
    METRICS_PORT=9090 \
    BACKEND_TARGETS=backend-1:9001,backend-2:9002,backend-3:9003

EXPOSE 50051 9090

ENTRYPOINT ["/usr/local/bin/controlplane"]
