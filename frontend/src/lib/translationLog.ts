import type { TranslationCaptionMessage, TranslationMessage, TranslationStatusMessage } from './webrtc/types'

export const maxTranslationCharacters = 400
export const maxTranslationEntries = 2_000
export const maxTranslationChannels = 16
export const maxTranslationTotalEntries = 4_000
export const translationLogRetentionMs = 60 * 60 * 1_000
export const translationDraftRetentionMs = 30_000

export interface TranslationLogEntry {
  id: string
  captionId: string
  originalText: string
  translatedText: string
  receivedAt: number
}

export interface TranslationLogDraft {
  captionId: string
  originalText: string
  translatedText: string
  updatedAt: number
}

export interface TranslationLogState {
  entries: TranslationLogEntry[]
  draft: TranslationLogDraft | null
  nextSequence: number
}

export type TranslationHistories = Record<string, TranslationLogState>
export type TranslationUIStatus = 'off' | 'connecting' | 'ready' | 'error'

export interface TranslationMessageRoute {
  channelId: string
  streamId: string
  establishesStream: boolean
}

export function emptyTranslationLog(): TranslationLogState {
  return { entries: [], draft: null, nextSequence: 0 }
}

export function emptyTranslationHistories(): TranslationHistories {
  return Object.create(null) as TranslationHistories
}

export function translationStatusClearsDraft(status: TranslationStatusMessage['status']): boolean {
  return status === 'unavailable' || status === 'error'
}

export function translationStatusAfterRestart(
  current: TranslationUIStatus,
  enabled: boolean,
): TranslationUIStatus {
  if (!enabled) return 'off'
  return current === 'ready' || current === 'error' ? current : 'connecting'
}

export function translationMessageRoute(
  message: TranslationMessage,
  targetChannelId: string,
  activeStreamId: string,
  retiredStreamIds: ReadonlySet<string>,
): TranslationMessageRoute | null {
  const channelId = message.channelId || ''
  const streamId = message.streamId || ''
  if (!channelId || channelId !== targetChannelId) return null
  if (!streamId) return null
  if (streamId && retiredStreamIds.has(streamId)) return null

  const ready = message.type === 'translation-status' && message.status === 'ready'
  if (ready) {
    if (activeStreamId && streamId !== activeStreamId) return null
    return { channelId, streamId, establishesStream: !activeStreamId }
  }
  if (activeStreamId && streamId !== activeStreamId) return null
  if (!activeStreamId && (message.type !== 'translation-status' || message.status !== 'error')) return null
  return { channelId, streamId, establishesStream: false }
}

function normalizedTranslationText(value: string | undefined): string {
  const normalized = (value ?? '')
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  return Array.from(normalized).slice(0, maxTranslationCharacters).join('')
}

function prunedLog(log: TranslationLogState, now: number): TranslationLogState | null {
  const cutoff = now - translationLogRetentionMs
  const firstRetained = log.entries.findIndex(entry => entry.receivedAt >= cutoff)
  const entries = firstRetained < 0 ? [] : firstRetained === 0 ? log.entries : log.entries.slice(firstRetained)
  const draft = log.draft && log.draft.updatedAt >= now - translationDraftRetentionMs ? log.draft : null
  if (!entries.length && !draft) return null
  if (entries === log.entries && draft === log.draft) return log
  return { ...log, entries, draft }
}

function updatedLog(
  current: TranslationLogState,
  message: TranslationCaptionMessage,
  now: number,
): TranslationLogState {
  const translatedText = normalizedTranslationText(message.text)
  if (!translatedText) return current
  const captionId = normalizedTranslationText(message.captionId)
  const originalText = normalizedTranslationText(message.originalText)

  if (message.phase === 'partial') {
    return {
      ...current,
      draft: { captionId, originalText, translatedText, updatedAt: now },
    }
  }

  const sequence = current.nextSequence + 1
  const matchingDraft = current.draft
    && (!captionId || !current.draft.captionId || current.draft.captionId === captionId)
    ? current.draft
    : null
  const entry: TranslationLogEntry = {
    id: `${captionId || 'translation'}:${sequence}`,
    captionId,
    originalText: originalText || matchingDraft?.originalText || '',
    translatedText,
    receivedAt: now,
  }
  return {
    entries: [...current.entries, entry].slice(-maxTranslationEntries),
    draft: null,
    nextSequence: sequence,
  }
}

export function translationHistoryState(
  histories: TranslationHistories,
  channelId: string,
  message: TranslationCaptionMessage | null,
  now: number,
): TranslationHistories {
  const next = pruneTranslationHistories(histories, now)
  if (!channelId || !message) return next
  next[channelId] = updatedLog(next[channelId] ?? emptyTranslationLog(), message, now)
  return boundedTranslationHistories(next)
}

export function pruneTranslationHistories(
  histories: TranslationHistories,
  now: number,
): TranslationHistories {
  const next = emptyTranslationHistories()
  for (const [key, log] of Object.entries(histories)) {
    const pruned = prunedLog(log, now)
    if (pruned) next[key] = pruned
  }
  return boundedTranslationHistories(next)
}

export function clearTranslationDraft(
  histories: TranslationHistories,
  channelId: string,
): TranslationHistories {
  const current = histories[channelId]
  if (!current?.draft) return histories
  const next = emptyTranslationHistories()
  Object.assign(next, histories)
  next[channelId] = { ...current, draft: null }
  return next
}

function logActivity(log: TranslationLogState): number {
  const finalAt = log.entries.length ? log.entries[log.entries.length - 1].receivedAt : 0
  return Math.max(finalAt, log.draft?.updatedAt ?? 0)
}

function boundedTranslationHistories(histories: TranslationHistories): TranslationHistories {
  const channels = Object.entries(histories)
    .sort((left, right) => logActivity(right[1]) - logActivity(left[1]))
    .slice(0, maxTranslationChannels)
  const next = emptyTranslationHistories()
  for (const [channelId, log] of channels) next[channelId] = log

  const totalEntries = channels.reduce((total, [, log]) => total + log.entries.length, 0)
  if (totalEntries <= maxTranslationTotalEntries) return next

  const newest = channels
    .flatMap(([channelId, log]) => log.entries.map(entry => ({ channelId, entry })))
    .sort((left, right) => right.entry.receivedAt - left.entry.receivedAt)
    .slice(0, maxTranslationTotalEntries)
  const retained = new Map<string, Set<string>>()
  for (const { channelId, entry } of newest) {
    const ids = retained.get(channelId) ?? new Set<string>()
    ids.add(entry.id)
    retained.set(channelId, ids)
  }
  for (const [channelId, log] of Object.entries(next)) {
    const ids = retained.get(channelId)
    next[channelId] = { ...log, entries: ids ? log.entries.filter(entry => ids.has(entry.id)) : [] }
  }
  return next
}
