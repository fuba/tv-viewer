# Deployment Guide

## Prerequisites

- Docker and Docker Compose installed
- Access to Mirakurun server (default: http://tuner:40772)
- Sufficient disk space for video segments
- Network connectivity for streaming

## Production Deployment

### 1. Clone the Repository

```bash
git clone https://github.com/fuba/tv-viewer.git
cd tv-viewer
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env file with your settings
vim .env
```

### 3. Build and Start Services

```bash
# Build production images
make prod-build

# Start services
make prod

# Or using docker-compose directly
docker compose up -d
```

### 4. Verify Installation

- Backend API: http://localhost:8080/api/health
- Frontend UI: http://localhost:3000

## Configuration

### Mirakurun Server

Set the Mirakurun server URL in the environment:

```bash
MIRAKURUN_URL=http://your-mirakurun-server:40772
```

### Ports

- Backend API: 8080 (configurable via PORT env)
- Frontend UI: 3000

### Storage

Video segments and subtitles are stored in `./stream` directory.
Ensure sufficient disk space for temporary segment storage.

## Monitoring

### View Logs

```bash
# All services
make logs

# Specific service
make logs-backend
make logs-frontend
```

### Health Check

```bash
curl http://localhost:8080/api/health
```

## Troubleshooting

### FFmpeg Issues

If encoding fails, check FFmpeg installation:

```bash
docker exec tv-viewer-backend-1 ffmpeg -version
```

### Mirakurun Connection

Test Mirakurun connectivity:

```bash
curl http://tuner:40772/api/channels
```

### Disk Space

Monitor disk usage for segment storage:

```bash
du -sh ./stream
```

## Security Considerations

- Use HTTPS in production
- Configure firewall rules
- Limit access to Mirakurun server
- Regular security updates

## Backup

Important data to backup:
- SQLite database: `./data/tv-viewer.db`
- Configuration files: `.env`

## Updates

```bash
git pull
make build
make up
```