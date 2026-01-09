// WebRTC signaling message types
export interface SignalingMessage {
  type: 'offer' | 'answer' | 'ice-candidate' | 'stream-start' | 'stream-stop' | 'error' | 'ready' | 'ping' | 'pong';
  peerId?: string;
  channelId?: string;
  sdp?: RTCSessionDescriptionInit;
  candidate?: RTCIceCandidateInit;
  error?: string;
  timestamp?: number;
}

// Subtitle message from DataChannel
export interface SubtitleMessage {
  type: 'show' | 'hide' | 'clear';
  id?: string;
  text?: string;
  startTime?: number;
  endTime?: number;
  style?: string;
}

// Connection status
export type ConnectionStatus =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'failed'
  | 'closed';

// RTCClient options
export interface RTCClientOptions {
  channelId: string;
  onTrack?: (track: MediaStreamTrack, stream: MediaStream) => void;
  onConnectionStateChange?: (state: ConnectionStatus) => void;
  onSubtitle?: (subtitle: SubtitleMessage) => void;
  onError?: (error: Error) => void;
  onLog?: (message: string) => void;
}
