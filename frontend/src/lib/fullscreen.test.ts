import assert from 'node:assert/strict'
import test from 'node:test'

import { createOrientationLockCoordinator, requestViewerFullscreen } from './fullscreen.ts'

test('fullscreen request only enters fullscreen', async () => {
  const calls: string[] = []
  await requestViewerFullscreen(
    { requestFullscreen: async () => { calls.push('fullscreen') } },
  )
  assert.deepEqual(calls, ['fullscreen'])
})

test('unsupported fullscreen request still succeeds', async () => {
  let entered = false
  await requestViewerFullscreen({ requestFullscreen: async () => { entered = true } })
  assert.equal(entered, true)
})

test('pending orientation lock is retained when fullscreen is immediately re-entered', async () => {
  const calls: string[] = []
  let resolveLock: (() => void) | undefined
  const coordinator = createOrientationLockCoordinator({
    lock: async (orientation) => {
      calls.push(orientation)
      await new Promise<void>(resolve => { resolveLock = resolve })
    },
    unlock: () => { calls.push('unlock') },
  })

  coordinator.setFullscreen(true)
  coordinator.setFullscreen(false)
  coordinator.setFullscreen(true)
  resolveLock?.()
  await coordinator.waitForIdle()

  assert.deepEqual(calls, ['landscape'])
})

test('orientation is unlocked once after leaving a completed fullscreen lock', async () => {
  const calls: string[] = []
  const coordinator = createOrientationLockCoordinator({
    lock: async (orientation) => { calls.push(orientation) },
    unlock: () => { calls.push('unlock') },
  })

  coordinator.setFullscreen(true)
  await coordinator.waitForIdle()
  coordinator.setFullscreen(false)

  assert.deepEqual(calls, ['landscape', 'unlock'])
})
