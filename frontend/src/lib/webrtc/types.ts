// Audio mode for dual mono streams (Japanese bilingual broadcasts)
export type AudioMode = 'main' | 'sub' | 'both';

import type { VideoFormatMessage } from '../videoFormat';

// WebRTC signaling message types
export interface SignalingMessage {
  type: 'offer' | 'answer' | 'ice-candidate' | 'stream-start' | 'stream-stop' | 'stream-stopped' | 'restart-encoding' | 'encoding-restarted' | 'error' | 'ready' | 'ping' | 'pong';
  peerId?: string;
  channelId?: string;
  requestId?: string;
  sdp?: RTCSessionDescriptionInit;
  candidate?: RTCIceCandidateInit;
  error?: string;
  timestamp?: number;
  burnInSubtitles?: boolean; // If true, ARIB captions are sent over the data channel
  audioMode?: AudioMode;     // Dual mono mode: 'main' (left), 'sub' (right), 'both' (stereo)
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

// Video geometry announced by the server, parsed from the MPEG-2 sequence header
export type { VideoFormatMessage } from '../videoFormat';

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
  burnInSubtitles?: boolean; // If true, ARIB captions are sent over the data channel
  audioMode?: AudioMode;     // Dual mono mode: 'main', 'sub', or 'both' (default: 'both')
  onTrack?: (track: MediaStreamTrack, stream: MediaStream) => void;
  onConnectionStateChange?: (state: ConnectionStatus) => void;
  onSubtitle?: (subtitle: SubtitleMessage) => void;
  onVideoFormat?: (format: VideoFormatMessage) => void; // Broadcast picture geometry (aspect ratio)
  onEncodingRestarted?: (channelId: string, requestId?: string) => void; // Called when encoding is restarted (e.g., subtitle toggle, channel change)
  onError?: (error: Error, requestId?: string) => void;
  onLog?: (message: string) => void;
}
