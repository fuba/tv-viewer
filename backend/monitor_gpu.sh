#!/bin/bash

echo "Monitoring GPU usage and FFmpeg processes..."
echo "============================================"

while true; do
    echo -n "$(date '+%Y-%m-%d %H:%M:%S') - "
    
    # Check GPU usage
    gpu_usage=$(nvidia-smi --query-gpu=utilization.gpu,utilization.encoder --format=csv,noheader,nounits | tr ',' ' ')
    echo -n "GPU: $gpu_usage% | "
    
    # Check FFmpeg processes in Docker
    ffmpeg_count=$(docker exec tv-viewer-backend-1 ps aux 2>/dev/null | grep -c ffmpeg | grep -v grep || echo 0)
    echo -n "FFmpeg processes: $ffmpeg_count | "
    
    # Check if using NVENC
    nvenc_active=$(docker exec tv-viewer-backend-1 ps aux 2>/dev/null | grep ffmpeg | grep -c nvenc || echo 0)
    if [ $nvenc_active -gt 0 ]; then
        echo "NVENC: ACTIVE"
    else
        echo "NVENC: INACTIVE"
    fi
    
    sleep 2
done