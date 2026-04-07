# Multi-stage build: frontend first, then Go binary, then minimal final image.

# ── Stage 1: Build the Svelte frontend ───────────────────────────────────────
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build
# Output: /app/frontend/build/

# ── Stage 2: Build the Go binary ─────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed the built frontend into the binary
COPY --from=frontend-builder /app/frontend/build ./frontend/build
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ── Stage 3: Minimal runtime image ───────────────────────────────────────────
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /app/server ./server
EXPOSE 8080
CMD ["./server"]
