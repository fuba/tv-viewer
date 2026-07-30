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

// One run of characters as ARIB placed it on the caption plane
export interface CaptionSpan {
  text: string;
  left: number;
  advance: number;    // distance to the next run, so a row paints as one box
  chars?: number;     // characters in the run: advance/chars is the drawn cell
  fontWidth: number;
  fontHeight: number;
  charSpace: number;
  color?: string;
  background?: string;
  opacity?: number;
  backgroundOpacity?: number;
}

// One drawn caption line. A row of small runs above another row is ruby.
export interface CaptionRow {
  text: string;
  bottom: number;
  spans: CaptionSpan[];
}

// Subtitle message from DataChannel
export interface SubtitleMessage {
  type: 'show' | 'hide' | 'clear';
  id?: string;
  text?: string;
  startTime?: number;
  endTime?: number;
  style?: string;
  plane?: { width: number; height: number }; // ARIB caption plane, e.g. 960x540
  rows?: CaptionRow[];
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
