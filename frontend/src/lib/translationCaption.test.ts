import assert from 'node:assert/strict'
import test from 'node:test'

import {
  maxTranslationCharacters,
  translationCaptionState,
  translationStatusAfterRestart,
  translationStatusClearsCaption,
} from './translationCaption.ts'

test('partial translations overwrite the live caption and final captions live longer', () => {
  const partial = translationCaptionState({
    type: 'translation-caption', phase: 'partial', text: '  こんにちは\u0000  世界  ',
  }, 1000)
  assert.deepEqual(partial, { text: 'こんにちは 世界', phase: 'partial', expiresAt: 4000 })

  const final = translationCaptionState({
    type: 'translation-caption', phase: 'final', text: '確定字幕', captionId: 'caption-1',
  }, 2000)
  assert.deepEqual(final, { text: '確定字幕', phase: 'final', expiresAt: 10000 })
})

test('translation captions are bounded before rendering', () => {
  const state = translationCaptionState({
    type: 'translation-caption', phase: 'final', text: '訳'.repeat(maxTranslationCharacters + 20),
  }, 0)
  assert.ok(state)
  assert.equal(Array.from(state.text).length, maxTranslationCharacters)
})

test('empty translation captions are ignored', () => {
  assert.equal(translationCaptionState({ type: 'translation-caption', phase: 'partial', text: ' \u0000 ' }, 0), null)
})

test('unavailable and error statuses clear stale translated captions', () => {
  assert.equal(translationStatusClearsCaption('unavailable'), true)
  assert.equal(translationStatusClearsCaption('error'), true)
  assert.equal(translationStatusClearsCaption('ready'), false)
  assert.equal(translationStatusClearsCaption('speaking'), false)
})

test('an encoding acknowledgement cannot fake VoiceTranslate readiness', () => {
  assert.equal(translationStatusAfterRestart('connecting', true), 'connecting')
  assert.equal(translationStatusAfterRestart('error', true), 'error')
  assert.equal(translationStatusAfterRestart('ready', true), 'ready')
  assert.equal(translationStatusAfterRestart('ready', false), 'off')
})
