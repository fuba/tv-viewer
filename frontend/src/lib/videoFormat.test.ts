import assert from 'node:assert/strict'
import test from 'node:test'

import { broadcastDisplayAspect, displayAspectRatio } from './videoFormat.ts'

test('display aspect follows the announced broadcast ratio', () => {
  assert.equal(displayAspectRatio({ aspectNum: 16, aspectDen: 9 }), 16 / 9)
  assert.equal(displayAspectRatio({ aspectNum: 4, aspectDen: 3 }), 4 / 3)
  // 16:9 over the 704 active pixels of a 720 wide SD picture.
  assert.equal(displayAspectRatio({ aspectNum: 20, aspectDen: 11 }), 20 / 11)
})

test('display aspect falls back to 16:9 until the server announces one', () => {
  assert.equal(displayAspectRatio(undefined), broadcastDisplayAspect)
  assert.equal(displayAspectRatio(null), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({}), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({ aspectNum: 16 }), broadcastDisplayAspect)
})

test('display aspect rejects ratios no broadcast can produce', () => {
  assert.equal(displayAspectRatio({ aspectNum: 0, aspectDen: 9 }), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({ aspectNum: -16, aspectDen: 9 }), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({ aspectNum: 16, aspectDen: 0 }), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({ aspectNum: 100, aspectDen: 1 }), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({ aspectNum: 1, aspectDen: 100 }), broadcastDisplayAspect)
  assert.equal(displayAspectRatio({ aspectNum: Number.NaN, aspectDen: 9 }), broadcastDisplayAspect)
})
