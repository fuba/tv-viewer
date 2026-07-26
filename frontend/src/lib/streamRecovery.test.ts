import assert from 'node:assert/strict'
import test from 'node:test'

import { reconnectDelay, shouldRecoverStream } from './streamRecovery.ts'

test('stream recovery uses bounded exponential backoff', () => {
  assert.equal(reconnectDelay(1), 1000)
  assert.equal(reconnectDelay(2), 2000)
  assert.equal(reconnectDelay(3), 4000)
  assert.equal(reconnectDelay(8), 10000)
})

test('stream recovery only handles unexpected terminal states', () => {
  assert.equal(shouldRecoverStream('failed', false, true), true)
  assert.equal(shouldRecoverStream('closed', false, true), true)
  assert.equal(shouldRecoverStream('connected', false, true), false)
  assert.equal(shouldRecoverStream('closed', true, true), false)
  assert.equal(shouldRecoverStream('closed', false, false), false)
})
