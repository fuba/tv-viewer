import assert from 'node:assert/strict'
import test from 'node:test'

import { fittedFullscreenVideoSize, fittedVideoWidth } from './viewportFit.ts'

test('viewport fit uses available height on a short desktop window', () => {
  assert.ok(Math.abs(fittedVideoWidth(1440, 768, 168) - 600 * 16 / 9) < 0.001)
})

test('viewport fit never exceeds the player container', () => {
  assert.equal(fittedVideoWidth(390, 844, 160), 390)
})

test('viewport fit accepts the narrower visual viewport while zoomed', () => {
  assert.equal(fittedVideoWidth(240, 500, 100), 240)
})

test('fullscreen video uses the full landscape height without reserving toolbar space', () => {
  const size = fittedFullscreenVideoSize(844, 390)
  assert.equal(size.height, 390)
  assert.ok(Math.abs(size.width - (390 * 16 / 9)) < 0.001)
})

test('fullscreen video remains 16:9 after returning to a portrait viewport', () => {
  const landscape = fittedFullscreenVideoSize(844, 390)
  const portrait = fittedFullscreenVideoSize(390, 844)

  assert.equal(landscape.width / landscape.height, 16 / 9)
  assert.equal(portrait.width / portrait.height, 16 / 9)
  assert.deepEqual(portrait, { width: 390, height: 219.375 })
})
