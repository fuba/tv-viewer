# Build stage
FROM golang:1.22-bookworm AS builder

RUN apt-get update && apt-get install -y \
    gcc \
    libsqlite3-dev \
    libmpeg2-4-dev \
    libfaad-dev \
    libopus-dev \
    libaribb24-dev \
    ca-certificates \
    wget \
    && rm -rf /var/lib/apt/lists/*

ARG NV_CODEC_HEADERS_URL=https://github.com/FFmpeg/nv-codec-headers/archive/refs/heads/sdk/13.0.tar.gz
ARG NV_CODEC_HEADERS_SHA256=a22c173534f6ef5c3125caeb5f06f2bcb1079803e8390a7817a6a3254785667d
RUN wget -q -O /tmp/nv-codec-headers.tar.gz "$NV_CODEC_HEADERS_URL" \
    && echo "$NV_CODEC_HEADERS_SHA256  /tmp/nv-codec-headers.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/nv-codec-headers.tar.gz -C /tmp \
    && mkdir -p /usr/local/include/ffnvcodec \
    && cp /tmp/nv-codec-headers-sdk-13.0/include/ffnvcodec/*.h /usr/local/include/ffnvcodec/ \
    && rm -rf /tmp/nv-codec-headers.tar.gz /tmp/nv-codec-headers-sdk-13.0

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -tags native -o server cmd/server/main.go

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libsqlite3-0 \
    libmpeg2-4 \
    libfaad2 \
    libopus0 \
    libaribb24-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 18088

CMD ["./server"]
