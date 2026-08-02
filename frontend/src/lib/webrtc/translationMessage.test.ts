import assert from 'node:assert/strict'
import test from 'node:test'

import { validatedTranslationMessage } from './translationMessage.ts'

test('valid translation messages pass the DataChannel boundary', () => {
  const message = {
    type: 'translation-caption', phase: 'final', captionId: 'caption-1',
    channelId: 'channel-1', streamId: 'stream-1', originalText: 'Hello', text: 'こんにちは',
  }
  assert.deepEqual(validatedTranslationMessage(message), message)
})

test('malformed and oversized translation messages are rejected', () => {
  assert.equal(validatedTranslationMessage({ type: 'translation-caption', phase: 'final', text: {} }), null)
  assert.equal(validatedTranslationMessage({ type: 'translation-caption', phase: 'other', text: '訳' }), null)
  assert.equal(validatedTranslationMessage({ type: 'translation-caption', phase: 'final', text: '訳'.repeat(401) }), null)
  assert.equal(validatedTranslationMessage({ type: 'translation-caption', phase: 'final', text: '訳' }), null)
  assert.equal(validatedTranslationMessage({ type: 'translation-status', status: 'invented' }), null)
  assert.equal(validatedTranslationMessage({ type: 'translation-status', status: 'error', message: {}, channelId: 'a', streamId: 'b' }), null)
})
