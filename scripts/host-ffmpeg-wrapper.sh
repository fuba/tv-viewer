#!/bin/bash
# FFmpeg wrapper script to use host FFmpeg from container
# This script runs on the host via SSH or direct execution

# Convert container paths to host paths
ARGS=("$@")
for i in "${!ARGS[@]}"; do
    # Replace /app/stream with host path
    ARGS[$i]=${ARGS[$i]//\/app\/stream/\/home\/ec\/tv-viewer\/stream}
done

# Execute host ffmpeg with NVENC support
exec /usr/bin/ffmpeg "${ARGS[@]}"