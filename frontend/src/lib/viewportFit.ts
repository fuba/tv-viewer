export function fittedVideoWidth(
  containerWidth: number,
  viewportHeight: number,
  occupiedHeight: number,
  aspectRatio = 16 / 9,
): number {
  const availableHeight = Math.max(0, viewportHeight - occupiedHeight)
  return Math.max(0, Math.min(containerWidth, availableHeight * aspectRatio))
}

export function fittedFullscreenVideoSize(
  viewportWidth: number,
  viewportHeight: number,
  aspectRatio = 16 / 9,
): { width: number, height: number } {
  const safeWidth = Math.max(0, viewportWidth)
  const safeHeight = Math.max(0, viewportHeight)
  const widthFromHeight = safeHeight * aspectRatio

  if (widthFromHeight <= safeWidth) {
    return { width: widthFromHeight, height: safeHeight }
  }
  return { width: safeWidth, height: safeWidth / aspectRatio }
}
