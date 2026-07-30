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
  minWidth: number        // % of the plane width, so a row paints as one box
  height: number          // % of the plane height
  fontSize: number        // % of the picture height (cqh)
  letterSpacing: number   // em
  scaleX: number
  color: string
  background: string
  opacity: number
  backgroundOpacity: number
}

const defaultColor = '#ffffff'
const defaultBackground = '#000000'

function ratio(value: number, total: number): number {
  if (!Number.isFinite(value) || !Number.isFinite(total) || total <= 0) return 0
  return (value / total) * 100
}

// A run has to fit the cells the broadcaster drew it in, and a rendering font
// is never as narrow as the broadcast one: two normal characters that ARIB
// advanced by 60 would take 80 in the browser and collide with the next run.
// The advance to the next run gives the true cell, so scale the glyphs to it.
export function horizontalScale(span: CaptionSpan): number {
  const fontHeight = span.fontHeight > 0 ? span.fontHeight : 0
  const glyph = fontHeight + span.charSpace
  if (!Number.isFinite(glyph) || glyph <= 0) return 1
  const chars = span.chars && span.chars > 0 ? span.chars : [...span.text].length
  if (span.advance > 0 && chars > 0) {
    return Math.min(1, span.advance / chars / glyph)
  }
  // The last run of a row has no next run: fall back to its own cell width.
  const cell = span.fontWidth + span.charSpace
  if (cell <= 0) return 1
  return Math.min(1, cell / glyph)
}

export function captionSpanStyle(span: CaptionSpan, row: CaptionRow, plane: CaptionPlane): CaptionSpanStyle {
  const fontHeight = span.fontHeight > 0 ? span.fontHeight : 0
  return {
    left: ratio(span.left, plane.width),
    bottom: ratio(plane.height - row.bottom, plane.height),
    minWidth: ratio(span.advance, plane.width),
    height: ratio(fontHeight, plane.height),
    fontSize: ratio(fontHeight, plane.height),
    letterSpacing: fontHeight > 0 ? span.charSpace / fontHeight : 0,
    scaleX: horizontalScale(span),
    color: span.color || defaultColor,
    background: span.background || defaultBackground,
    opacity: typeof span.opacity === 'number' ? span.opacity : 1,
    backgroundOpacity: typeof span.backgroundOpacity === 'number' ? span.backgroundOpacity : 1,
  }
}

export function hasPlacedLayout(caption: { plane?: CaptionPlane, rows?: CaptionRow[] } | null | undefined): boolean {
  const plane = caption?.plane
  return Boolean(plane && plane.width > 0 && plane.height > 0 && caption?.rows?.length)
}
