# Stage 1: Build Rust Data Plane with protoc
FROM rust:1.80-bullseye AS builder

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy proto contracts and dataplane sources
COPY proto/ /workspace/proto/
COPY dataplane/ /workspace/dataplane/

WORKDIR /workspace/dataplane
RUN cargo build --release

# Stage 2: Minimal runtime image
FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /workspace/dataplane/target/release/dataplane /usr/local/bin/dataplane

ENV PROXY_LISTEN_ADDR=0.0.0.0:8080 \
    CONTROLLER_ADDR=http://controlplane:50051 \
    PROXY_NODE_ID=node-docker-01

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/dataplane"]
