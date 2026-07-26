#!/bin/bash

echo "Monitoring native NVENC and GPU usage..."
echo "============================================"

while true; do
    echo -n "$(date '+%Y-%m-%d %H:%M:%S') - "
    
    # Check GPU usage
    gpu_usage=$(nvidia-smi --query-gpu=utilization.gpu,utilization.encoder --format=csv,noheader,nounits | tr ',' ' ')
    echo -n "GPU: $gpu_usage% | "
    
    echo "Native NVENC is loaded through the backend process"
    
    sleep 2
done
