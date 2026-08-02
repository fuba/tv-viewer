// Broadcasts code non-square pixels: 1440x1080 (SAR 4:3) and 720x480 (SAR
// 32:27) are both 16:9 pictures. WebRTC hands the browser the coded size with
// no aspect information, so the ratio comes from the MPEG-2 sequence header the
// server parses and announces over the data channel.
export const broadcastDisplayAspect = 16 / 9

export interface VideoFormatMessage {
  type: 'video-format'
  width?: number
  height?: number
  aspectNum?: number
  aspectDen?: number
}

// Anything outside this range is a decoding accident, not a broadcast.
const minimumAspect = 0.5
const maximumAspect = 4

export function displayAspectRatio(message: Partial<VideoFormatMessage> | null | undefined): number {
  const num = message?.aspectNum
  const den = message?.aspectDen
  if (typeof num !== 'number' || typeof den !== 'number') return broadcastDisplayAspect
  if (!Number.isFinite(num) || !Number.isFinite(den) || num <= 0 || den <= 0) return broadcastDisplayAspect
  const ratio = num / den
  if (ratio < minimumAspect || ratio > maximumAspect) return broadcastDisplayAspect
  return ratio
}
