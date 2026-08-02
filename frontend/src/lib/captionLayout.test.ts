import assert from 'node:assert/strict'
import test from 'node:test'

import {
  captionBackgroundOpacity,
  captionBands,
  captionSpanStyle,
  hasPlacedLayout,
  horizontalScale,
} from './captionLayout.ts'
import type { CaptionRow, CaptionSpan } from './webrtc/types.ts'

// Captured from a live NHK Eテレ broadcast: "（筒井）うん！" drawn on the lower
// row of a 960x540 plane, opening bracket in the half width character size.
const plane = { width: 960, height: 540 }
// Rows report the lower edge of their character blocks and the full block
// height, so a two row caption paints as one black band.
const row: CaptionRow = { text: '（筒井）うん！', bottom: 509, height: 60, spans: [] }
const bracket: CaptionSpan = {
  text: '（', left: 658, width: 20, chars: 1, fontWidth: 18, fontHeight: 36, charSpace: 2,
}
const name: CaptionSpan = {
  text: '筒井', left: 678, width: 80, chars: 2, fontWidth: 36, fontHeight: 36, charSpace: 4,
}

test('a span keeps the position the broadcaster drew it at', () => {
  const style = captionSpanStyle(name, row, plane)
  assert.equal(style.left, (678 / 960) * 100)
  // 509 of 540 down the plane leaves the row just above the bottom edge.
  assert.equal(style.bottom, ((540 - 509) / 540) * 100)
  assert.equal(style.fontSize, (36 / 540) * 100)
})

test('the drawn box covers whole character blocks', () => {
  const style = captionSpanStyle(name, row, plane)
  // Two normal blocks wide, and a full block high - glyph plus line spacing.
  assert.equal(style.width, (80 / 960) * 100)
  assert.equal(style.height, (60 / 540) * 100)
  // That margin is what makes the glyphs sit inside the box instead of filling it.
  assert.ok(style.height > style.fontSize)
})

test('adjacent runs tile without overlapping', () => {
  const first = captionSpanStyle(bracket, row, plane)
  const second = captionSpanStyle(name, row, plane)
  assert.equal(first.left + first.width, second.left)
})

test('only half width runs are squeezed', () => {
  // A half width run draws a narrow glyph in a narrow cell.
  assert.equal(horizontalScale(bracket), 0.5)
  // A normal run fills its own cell and must not be distorted.
  assert.equal(horizontalScale(name), 1)
  // Ruby is small in both directions, so it is not squeezed either.
  assert.equal(horizontalScale({ ...name, fontWidth: 18, fontHeight: 18, charSpace: 2 }), 1)
})

test('character spacing scales with the character size', () => {
  assert.equal(captionSpanStyle(name, row, plane).letterSpacing, 4 / 36)
  assert.equal(captionSpanStyle(bracket, row, plane).letterSpacing, 2 / 36)
})

test('ruby is placed above its base text', () => {
  const base: CaptionSpan = { text: '漢字', left: 200, width: 80, fontWidth: 36, fontHeight: 36, charSpace: 4 }
  const ruby: CaptionSpan = { text: 'かんじ', left: 200, width: 60, fontWidth: 18, fontHeight: 18, charSpace: 2 }
  const baseStyle = captionSpanStyle(base, { text: '漢字', bottom: 509, height: 60, spans: [] }, plane)
  const rubyStyle = captionSpanStyle(ruby, { text: 'かんじ', bottom: 473, height: 30, spans: [] }, plane)

  assert.ok(rubyStyle.bottom > baseStyle.bottom, 'ruby must sit higher than its base text')
  assert.ok(rubyStyle.fontSize < baseStyle.fontSize, 'ruby must be drawn smaller')
  assert.equal(rubyStyle.left, baseStyle.left)
})

test('colours fall back to the usual white text', () => {
  const style = captionSpanStyle(bracket, row, plane)
  assert.equal(style.color, '#ffffff')
  assert.equal(style.opacity, 1)

  const coloured = captionSpanStyle({ ...bracket, color: '#00a0ff', opacity: 0.5 }, row, plane)
  assert.equal(coloured.color, '#00a0ff')
  assert.equal(coloured.opacity, 0.5)
})

test('touching runs paint as one band, so a translucent background has no seams', () => {
  const bands = captionBands({ ...row, spans: [bracket, name] }, plane)
  assert.equal(bands.length, 1)
  assert.equal(bands[0].left, (658 / 960) * 100)
  assert.equal(bands[0].width, ((20 + 80) / 960) * 100)
  assert.equal(bands[0].height, (60 / 540) * 100)
})

test('a gap the broadcaster left keeps the picture visible', () => {
  const detached: CaptionSpan = { ...name, left: 758, width: 80 }
  const bands = captionBands({ ...row, spans: [bracket, detached] }, plane)
  assert.equal(bands.length, 2)
  assert.equal(bands[1].left, (758 / 960) * 100)
})

test('the background is translucent but keeps what the broadcast asked for', () => {
  const bands = captionBands({ ...row, spans: [bracket] }, plane)
  assert.equal(bands[0].background, '#000000')
  assert.equal(bands[0].opacity, captionBackgroundOpacity)
  assert.ok(captionBackgroundOpacity > 0 && captionBackgroundOpacity < 1)

  // A background the broadcaster made transparent must not be painted at all.
  assert.deepEqual(captionBands({ ...row, spans: [{ ...bracket, backgroundOpacity: 0 }] }, plane), [])

  // A different background colour starts its own band.
  const coloured = captionBands(
    { ...row, spans: [bracket, { ...name, background: '#0000ff' }] },
    plane,
  )
  assert.equal(coloured.length, 2)
  assert.equal(coloured[1].background, '#0000ff')
})

test('a caption without geometry falls back to the plain text layout', () => {
  assert.equal(hasPlacedLayout({ plane, rows: [row] }), true)
  assert.equal(hasPlacedLayout({ rows: [row] }), false)
  assert.equal(hasPlacedLayout({ plane, rows: [] }), false)
  assert.equal(hasPlacedLayout({ plane: { width: 0, height: 0 }, rows: [row] }), false)
  assert.equal(hasPlacedLayout(null), false)
})
