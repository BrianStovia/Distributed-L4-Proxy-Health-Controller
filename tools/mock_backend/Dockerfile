# Stage 1: Build mock backend
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY tools/mock_backend/main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/mock_backend main.go

# Stage 2: Scratch runtime container
FROM scratch

COPY --from=builder /bin/mock_backend /mock_backend

ENV PORT=9001 \
    BACKEND_ID=backend-1

EXPOSE 9001

ENTRYPOINT ["/mock_backend"]
