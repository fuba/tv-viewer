# Build stage
FROM golang:1.21-bookworm AS builder

RUN apt-get update && apt-get install -y \
    gcc \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o server cmd/server/main.go

# Runtime stage  
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    wget \
    xz-utils \
    libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/* \
    && wget -O ffmpeg.tar.xz "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz" \
    && tar -xf ffmpeg.tar.xz --strip-components=1 \
    && mv ffmpeg /usr/local/bin/ \
    && mv ffprobe /usr/local/bin/ \
    && rm -rf ffmpeg* \
    && chmod +x /usr/local/bin/ffmpeg \
    && chmod +x /usr/local/bin/ffprobe

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]