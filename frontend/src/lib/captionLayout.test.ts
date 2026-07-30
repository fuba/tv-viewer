import assert from 'node:assert/strict'
import test from 'node:test'

import { captionSpanStyle, hasPlacedLayout, horizontalScale } from './captionLayout.ts'
import type { CaptionRow, CaptionSpan } from './webrtc/types.ts'

// Captured from a live NHK Eテレ broadcast: "（筒井）うん！" drawn on the lower
// row of a 960x540 plane, opening bracket in the half width character size.
const plane = { width: 960, height: 540 }
const row: CaptionRow = { text: '（筒井）うん！', bottom: 508, spans: [] }
const bracket: CaptionSpan = {
  text: '（', left: 678, advance: 40, fontWidth: 18, fontHeight: 36, charSpace: 2,
}
const name: CaptionSpan = {
  text: '筒井', left: 718, advance: 60, fontWidth: 36, fontHeight: 36, charSpace: 4,
}

test('a span keeps the position the broadcaster drew it at', () => {
  const style = captionSpanStyle(name, row, plane)
  assert.equal(style.left, (718 / 960) * 100)
  // 508 of 540 down the plane leaves the row just above the bottom edge.
  assert.equal(style.bottom, ((540 - 508) / 540) * 100)
  assert.equal(style.fontSize, (36 / 540) * 100)
})

test('the advance paints a row as one unbroken box', () => {
  assert.equal(captionSpanStyle(bracket, row, plane).minWidth, (40 / 960) * 100)
  // The last span of a row claims no advance and is only as wide as its text.
  assert.equal(captionSpanStyle({ ...name, advance: 0 }, row, plane).minWidth, 0)
})

test('runs are scaled to the cells the broadcast drew them in', () => {
  // Two normal characters advanced by 60 must not render 80 wide and collide.
  assert.equal(horizontalScale({ ...name, advance: 60, chars: 2 }), 30 / 40)
  // A run followed by a gap keeps its natural size instead of stretching.
  assert.equal(horizontalScale({ ...name, advance: 100, chars: 2 }), 1)
  // The last run of a row has no advance and falls back to its own cell.
  assert.equal(horizontalScale({ ...bracket, advance: 0 }), 20 / 38)
  assert.equal(horizontalScale({ ...name, advance: 0 }), 1)
  // Ruby is small in both directions, so it is not squeezed either.
  assert.equal(horizontalScale({ ...name, fontWidth: 18, fontHeight: 18, charSpace: 2, advance: 0 }), 1)
})

test('character spacing scales with the character size', () => {
  assert.equal(captionSpanStyle(name, row, plane).letterSpacing, 4 / 36)
  assert.equal(captionSpanStyle(bracket, row, plane).letterSpacing, 2 / 36)
})

test('ruby is placed above its base text', () => {
  const base: CaptionSpan = { text: '漢字', left: 200, advance: 0, fontWidth: 36, fontHeight: 36, charSpace: 4 }
  const ruby: CaptionSpan = { text: 'かんじ', left: 200, advance: 0, fontWidth: 18, fontHeight: 18, charSpace: 2 }
  const baseStyle = captionSpanStyle(base, { text: '漢字', bottom: 508, spans: [] }, plane)
  const rubyStyle = captionSpanStyle(ruby, { text: 'かんじ', bottom: 472, spans: [] }, plane)

  assert.ok(rubyStyle.bottom > baseStyle.bottom, 'ruby must sit higher than its base text')
  assert.ok(rubyStyle.fontSize < baseStyle.fontSize, 'ruby must be drawn smaller')
  assert.equal(rubyStyle.left, baseStyle.left)
})

test('colours fall back to the usual white on black', () => {
  const style = captionSpanStyle(bracket, row, plane)
  assert.equal(style.color, '#ffffff')
  assert.equal(style.background, '#000000')
  assert.equal(style.opacity, 1)
  assert.equal(style.backgroundOpacity, 1)

  const coloured = captionSpanStyle(
    { ...bracket, color: '#00a0ff', background: '#202020', opacity: 0.5, backgroundOpacity: 0 },
    row,
    plane,
  )
  assert.equal(coloured.color, '#00a0ff')
  assert.equal(coloured.background, '#202020')
  assert.equal(coloured.opacity, 0.5)
  assert.equal(coloured.backgroundOpacity, 0)
})

test('a caption without geometry falls back to the plain text layout', () => {
  assert.equal(hasPlacedLayout({ plane, rows: [row] }), true)
  assert.equal(hasPlacedLayout({ rows: [row] }), false)
  assert.equal(hasPlacedLayout({ plane, rows: [] }), false)
  assert.equal(hasPlacedLayout({ plane: { width: 0, height: 0 }, rows: [row] }), false)
  assert.equal(hasPlacedLayout(null), false)
})
