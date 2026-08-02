import type { TranslationMessage } from './types'

const translationStatuses = new Set([
  'ready', 'speaking', 'speech-cancelled', 'unavailable', 'error', 'unknown',
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function validOptionalString(message: Record<string, unknown>, key: string, maximum: number): boolean {
  const value = message[key]
  return value === undefined || (typeof value === 'string' && Array.from(value).length <= maximum)
}

function validRequiredString(message: Record<string, unknown>, key: string, maximum: number): boolean {
  const value = message[key]
  return typeof value === 'string' && Boolean(value) && Array.from(value).length <= maximum
}

export function validatedTranslationMessage(value: unknown): TranslationMessage | null {
  if (!isRecord(value)) return null
  if (value.type === 'translation-caption') {
    if (value.phase !== 'partial' && value.phase !== 'final') return null
    if (typeof value.text !== 'string' || !value.text || Array.from(value.text).length > 400) return null
    const valid = validOptionalString(value, 'id', 256)
      && validOptionalString(value, 'captionId', 256)
      && validOptionalString(value, 'originalText', 400)
      && validOptionalString(value, 'sourceLanguage', 32)
      && validOptionalString(value, 'targetLanguage', 32)
      && validRequiredString(value, 'channelId', 256)
      && validRequiredString(value, 'streamId', 256)
    return valid ? value as unknown as TranslationMessage : null
  }
  if (value.type === 'translation-status') {
    if (typeof value.status !== 'string' || !translationStatuses.has(value.status)) return null
    const valid = validOptionalString(value, 'captionId', 256)
      && validOptionalString(value, 'stage', 64)
      && validOptionalString(value, 'message', 512)
      && validOptionalString(value, 'speaker', 128)
      && validRequiredString(value, 'channelId', 256)
      && validRequiredString(value, 'streamId', 256)
    return valid ? value as unknown as TranslationMessage : null
  }
  return null
}
