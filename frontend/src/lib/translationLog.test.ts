import assert from 'node:assert/strict'
import test from 'node:test'

import {
  clearTranslationDraft,
  maxTranslationChannels,
  maxTranslationEntries,
  maxTranslationTotalEntries,
  pruneTranslationHistories,
  translationDraftRetentionMs,
  translationHistoryState,
  translationLogRetentionMs,
  translationMessageRoute,
  translationStatusAfterRestart,
  translationStatusClearsDraft,
  type TranslationHistories,
} from './translationLog.ts'

test('partial translation updates one draft and final translation appends one history entry', () => {
  let histories: TranslationHistories = {}
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'partial', captionId: 'caption-1',
    originalText: '  Hello\u0000   world  ', text: '  こんにちは  世界  ',
  }, 1_000)
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'partial', captionId: 'caption-1',
    originalText: 'Hello world.', text: 'こんにちは、世界。',
  }, 2_000)

  assert.deepEqual(histories['channel-a']?.draft, {
    captionId: 'caption-1', originalText: 'Hello world.', translatedText: 'こんにちは、世界。', updatedAt: 2_000,
  })
  assert.equal(histories['channel-a']?.entries.length, 0)

  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'final', captionId: 'caption-1',
    originalText: 'Hello world.', text: 'こんにちは、世界。',
  }, 3_000)
  assert.deepEqual(histories['channel-a'], {
    entries: [{
      id: 'caption-1:1', captionId: 'caption-1', originalText: 'Hello world.',
      translatedText: 'こんにちは、世界。', receivedAt: 3_000,
    }],
    draft: null,
    nextSequence: 1,
  })
})

test('history is isolated per channel and entries older than one hour are removed', () => {
  let histories: TranslationHistories = {}
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'final', captionId: 'a', originalText: 'A', text: '訳A',
  }, 1_000)
  histories = translationHistoryState(histories, 'channel-b', {
    type: 'translation-caption', phase: 'final', captionId: 'b', originalText: 'B', text: '訳B',
  }, 2_000)
  histories = translationHistoryState(histories, 'channel-a', null, 1_000 + translationLogRetentionMs + 1)

  assert.equal(histories['channel-a'], undefined)
  assert.equal(histories['channel-b']?.entries[0]?.translatedText, '訳B')
})

test('a stale partial draft expires without removing final history', () => {
  let histories = translationHistoryState({}, 'channel-a', {
    type: 'translation-caption', phase: 'final', captionId: 'final', originalText: 'Done', text: '完了',
  }, 1_000)
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'partial', originalText: 'Live', text: '途中',
  }, 2_000)
  histories = translationHistoryState(histories, '', null, 2_000 + translationDraftRetentionMs + 1)

  assert.equal(histories['channel-a']?.draft, null)
  assert.equal(histories['channel-a']?.entries.length, 1)
})

test('final translation can inherit source text from the one active draft', () => {
  let histories = translationHistoryState({}, 'channel-a', {
    type: 'translation-caption', phase: 'partial', originalText: 'Hello', text: 'こんに',
  }, 1_000)
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'final', captionId: 'caption-1', text: 'こんにちは',
  }, 2_000)

  assert.equal(histories['channel-a']?.entries[0]?.originalText, 'Hello')
})

test('final translation does not inherit a draft with a different caption ID', () => {
  let histories = translationHistoryState({}, 'channel-a', {
    type: 'translation-caption', phase: 'partial', captionId: 'caption-a', originalText: 'Wrong', text: '途中',
  }, 1_000)
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'final', captionId: 'caption-b', text: '確定',
  }, 2_000)

  assert.equal(histories['channel-a']?.entries[0]?.originalText, '')
})

test('translation text is normalized and bounded before rendering', () => {
  const longText = ` \u0000 ${'訳'.repeat(420)} `
  const histories = translationHistoryState({}, 'channel-a', {
    type: 'translation-caption', phase: 'final', originalText: longText, text: longText,
  }, 1_000)
  const entry = histories['channel-a']?.entries[0]

  assert.ok(entry)
  assert.equal(Array.from(entry.originalText).length, 400)
  assert.equal(Array.from(entry.translatedText).length, 400)
})

test('history has a defensive per-channel entry cap', () => {
  let histories: TranslationHistories = {}
  for (let index = 0; index < maxTranslationEntries + 5; index += 1) {
    histories = translationHistoryState(histories, 'channel-a', {
      type: 'translation-caption', phase: 'final', captionId: String(index), originalText: 'A', text: '訳',
    }, index)
  }

  assert.equal(histories['channel-a']?.entries.length, maxTranslationEntries)
  assert.equal(histories['channel-a']?.entries[0]?.captionId, '5')
})

test('history has defensive total channel and entry caps', () => {
  let histories: TranslationHistories = {}
  for (let channel = 0; channel < maxTranslationChannels + 2; channel += 1) {
    histories = translationHistoryState(histories, `channel-${channel}`, {
      type: 'translation-caption', phase: 'final', captionId: String(channel), originalText: 'A', text: '訳',
    }, channel + 1)
  }
  assert.equal(Object.keys(histories).length, maxTranslationChannels)
  assert.equal(histories['channel-0'], undefined)
  assert.ok(histories[`channel-${maxTranslationChannels + 1}`])

  const crowded: TranslationHistories = {}
  for (let channel = 0; channel < 3; channel += 1) {
    crowded[`crowded-${channel}`] = {
      entries: Array.from({ length: maxTranslationEntries }, (_, index) => ({
        id: `${channel}:${index}`, captionId: String(index), originalText: 'A', translatedText: '訳',
        receivedAt: channel * maxTranslationEntries + index,
      })),
      draft: null,
      nextSequence: maxTranslationEntries,
    }
  }
  const bounded = pruneTranslationHistories(crowded, 10_000)
  const total = Object.values(bounded).reduce((count, log) => count + log.entries.length, 0)
  assert.equal(total, maxTranslationTotalEntries)
})

test('special channel keys do not modify the history object prototype', () => {
  const histories = translationHistoryState({}, '__proto__', {
    type: 'translation-caption', phase: 'final', originalText: 'A', text: '訳',
  }, 1_000)
  assert.equal(Object.getPrototypeOf(histories), null)
  assert.equal(histories['__proto__']?.entries.length, 1)
})

test('turning translation off or receiving an error clears only the draft', () => {
  let histories = translationHistoryState({}, 'channel-a', {
    type: 'translation-caption', phase: 'final', captionId: 'final', originalText: 'Done', text: '完了',
  }, 1_000)
  histories = translationHistoryState(histories, 'channel-a', {
    type: 'translation-caption', phase: 'partial', captionId: 'draft', originalText: 'Live', text: '途中',
  }, 2_000)
  histories = clearTranslationDraft(histories, 'channel-a')

  assert.equal(histories['channel-a']?.draft, null)
  assert.equal(histories['channel-a']?.entries.length, 1)
  assert.equal(translationStatusClearsDraft('error'), true)
  assert.equal(translationStatusClearsDraft('unavailable'), true)
  assert.equal(translationStatusClearsDraft('ready'), false)
})

test('an encoding acknowledgement cannot fake VoiceTranslate readiness', () => {
  assert.equal(translationStatusAfterRestart('connecting', true), 'connecting')
  assert.equal(translationStatusAfterRestart('error', true), 'error')
  assert.equal(translationStatusAfterRestart('ready', true), 'ready')
  assert.equal(translationStatusAfterRestart('ready', false), 'off')
})

test('stream identity prevents delayed translation from crossing a channel restart', () => {
  const retired = new Set(['stream-a'])
  const delayed = {
    type: 'translation-caption' as const, phase: 'final' as const, text: '古い訳',
    channelId: 'channel-a', streamId: 'stream-a',
  }
  assert.equal(translationMessageRoute(delayed, 'channel-b', '', retired), null)

  const ready = {
    type: 'translation-status' as const, status: 'ready' as const,
    channelId: 'channel-b', streamId: 'stream-b',
  }
  assert.deepEqual(translationMessageRoute(ready, 'channel-b', '', retired), {
    channelId: 'channel-b', streamId: 'stream-b', establishesStream: true,
  })
  assert.equal(translationMessageRoute({ ...ready, streamId: undefined }, 'channel-b', '', retired), null)

  const current = { ...delayed, channelId: 'channel-b', streamId: 'stream-b', text: '新しい訳' }
  assert.deepEqual(translationMessageRoute(current, 'channel-b', 'stream-b', retired), {
    channelId: 'channel-b', streamId: 'stream-b', establishesStream: false,
  })
  assert.equal(translationMessageRoute({ ...ready, streamId: 'stream-c' }, 'channel-b', 'stream-b', retired), null)
})
