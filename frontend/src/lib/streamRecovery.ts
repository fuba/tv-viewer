import type { ConnectionStatus } from './webrtc/types'

const maximumReconnectDelay = 10_000

export function reconnectDelay(attempt: number): number {
  return Math.min(1000 * (2 ** Math.max(0, attempt - 1)), maximumReconnectDelay)
}

export function shouldRecoverStream(
  state: ConnectionStatus,
  intentionallyStopped: boolean,
  hasSelectedChannel: boolean,
): boolean {
  return hasSelectedChannel && !intentionallyStopped && (state === 'failed' || state === 'closed')
}
