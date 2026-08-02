import type { TranslationCaptionMessage } from './webrtc/types'
import type { TranslationStatusMessage } from './webrtc/types'

export const maxTranslationCharacters = 400

export interface TranslationCaptionState {
  text: string
  phase: 'partial' | 'final'
  expiresAt: number
}

export type TranslationUIStatus = 'off' | 'connecting' | 'ready' | 'error'

export function translationStatusClearsCaption(status: TranslationStatusMessage['status']): boolean {
  return status === 'unavailable' || status === 'error'
}

export function translationStatusAfterRestart(
  current: TranslationUIStatus,
  enabled: boolean,
): TranslationUIStatus {
  if (!enabled) return 'off'
  return current === 'ready' || current === 'error' ? current : 'connecting'
}

// Bound and normalize untrusted inference output before it reaches the overlay.
export function translationCaptionState(
  message: TranslationCaptionMessage,
  now: number,
): TranslationCaptionState | null {
  const normalized = message.text
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  const text = Array.from(normalized).slice(0, maxTranslationCharacters).join('')
  if (!text) return null
  return {
    text,
    phase: message.phase,
    expiresAt: now + (message.phase === 'final' ? 8000 : 3000),
  }
}
