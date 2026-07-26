import assert from 'node:assert/strict'
import test from 'node:test'

import { normalizedVolume } from './audioVolume.ts'

test('volume is clamped to the HTML media range', () => {
  assert.equal(normalizedVolume(-0.2), 0)
  assert.equal(normalizedVolume(0.45), 0.45)
  assert.equal(normalizedVolume(1.2), 1)
})
