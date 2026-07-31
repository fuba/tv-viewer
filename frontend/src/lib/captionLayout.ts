import type { CaptionRow, CaptionSpan } from './webrtc/types'

// ARIB places every run of characters at an absolute position on a fixed caption
// plane (960x540 for HD). Reproducing that placement is what preserves the
// broadcaster's line breaks, ruby - a small run drawn above its base text - and
// where on the picture the caption belongs.
export interface CaptionPlane {
  width: number
  height: number
}

export interface CaptionSpanStyle {
  left: number            // % of the plane width
  bottom: number          // % of the plane height
  width: number           // % of the plane width: the drawn character blocks
  height: number          // % of the plane height: the full block height
  fontSize: number        // % of the picture height (cqh)
  letterSpacing: number   // em
  scaleX: number
  color: string
  opacity: number
}

// One painted stretch of caption background, covering the character blocks of
// the runs it spans. Runs are painted as one band rather than one box each, so
// a translucent background has no seams where the runs meet.
export interface CaptionBandStyle {
  left: number
  bottom: number
  width: number
  height: number
  background: string
  opacity: number
}

const defaultColor = '#ffffff'
const defaultBackground = '#000000'

// Broadcasts ask for an opaque background. Keeping the picture visible through
// it is a viewing preference, so the requested opacity is scaled rather than
// replaced: a background the broadcaster made transparent stays transparent.
export const captionBackgroundOpacity = 0.65

function ratio(value: number, total: number): number {
  if (!Number.isFinite(value) || !Number.isFinite(total) || total <= 0) return 0
  return (value / total) * 100
}

// A glyph is drawn at the character height, so a half width run - where ARIB
// draws a narrow glyph in a narrow cell - has to be squeezed to its own width.
export function horizontalScale(span: CaptionSpan): number {
  if (span.fontHeight <= 0 || span.fontWidth <= 0) return 1
  return Math.min(1, span.fontWidth / span.fontHeight)
}

// The background of an ARIB caption covers whole character blocks - the glyph
// plus the character and line spacing around it - which is why the drawn box
// carries a margin and why consecutive rows join into one black band.
export function captionSpanStyle(span: CaptionSpan, row: CaptionRow, plane: CaptionPlane): CaptionSpanStyle {
  const fontHeight = span.fontHeight > 0 ? span.fontHeight : 0
  const blockHeight = row.height > 0 ? row.height : fontHeight
  return {
    left: ratio(span.left, plane.width),
    bottom: ratio(plane.height - row.bottom, plane.height),
    width: ratio(span.width, plane.width),
    height: ratio(blockHeight, plane.height),
    fontSize: ratio(fontHeight, plane.height),
    letterSpacing: fontHeight > 0 ? span.charSpace / fontHeight : 0,
    scaleX: horizontalScale(span),
    color: span.color || defaultColor,
    opacity: typeof span.opacity === 'number' ? span.opacity : 1,
  }
}

// captionBands merges the runs of a row into the stretches of background the
// caption is written on: runs that touch and share a background are one band.
export function captionBands(row: CaptionRow, plane: CaptionPlane): CaptionBandStyle[] {
  const blockHeight = row.height > 0 ? row.height : 0
  const bands: CaptionBandStyle[] = []
  let current: { left: number, right: number, background: string, opacity: number } | null = null

  for (const span of row.spans ?? []) {
    const background = span.background || defaultBackground
    const opacity = (typeof span.backgroundOpacity === 'number' ? span.backgroundOpacity : 1)
      * captionBackgroundOpacity
    if (current && span.left <= current.right && background === current.background && opacity === current.opacity) {
      current.right = Math.max(current.right, span.left + span.width)
      continue
    }
    if (current) bands.push(bandStyle(current, row, plane, blockHeight))
    current = { left: span.left, right: span.left + span.width, background, opacity }
  }
  if (current) bands.push(bandStyle(current, row, plane, blockHeight))
  return bands.filter(band => band.width > 0 && band.opacity > 0)
}

function bandStyle(
  band: { left: number, right: number, background: string, opacity: number },
  row: CaptionRow,
  plane: CaptionPlane,
  blockHeight: number,
): CaptionBandStyle {
  return {
    left: ratio(band.left, plane.width),
    bottom: ratio(plane.height - row.bottom, plane.height),
    width: ratio(band.right - band.left, plane.width),
    height: ratio(blockHeight, plane.height),
    background: band.background,
    opacity: band.opacity,
  }
}

export function hasPlacedLayout(caption: { plane?: CaptionPlane, rows?: CaptionRow[] } | null | undefined): boolean {
  const plane = caption?.plane
  return Boolean(plane && plane.width > 0 && plane.height > 0 && caption?.rows?.length)
}
