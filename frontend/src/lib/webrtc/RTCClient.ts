import type { SignalingMessage, SubtitleMessage, ConnectionStatus, RTCClientOptions, AudioMode } from './types';

export class RTCClient {
  private pc: RTCPeerConnection | null = null;
  private ws: WebSocket | null = null;
  private channelId: string;
  private burnInSubtitles: boolean;
  private audioMode: AudioMode;
  private peerId: string | null = null;
  private mediaStream: MediaStream | null = null;
  private dataChannel: RTCDataChannel | null = null;
  private pingInterval: ReturnType<typeof setInterval> | null = null;
  private _isDisconnecting = false;

  private onTrack?: (track: MediaStreamTrack, stream: MediaStream) => void;
  private onConnectionStateChange?: (state: ConnectionStatus) => void;
  private onSubtitle?: (subtitle: SubtitleMessage) => void;
  private onEncodingRestarted?: (channelId: string, requestId?: string) => void;
  private onError?: (error: Error, requestId?: string) => void;
  private onLog?: (message: string) => void;
  private pendingRestarts = new Map<string, { channelId: string; burnInSubtitles: boolean; audioMode: AudioMode }>();

  constructor(options: RTCClientOptions) {
    this.channelId = options.channelId;
    // Display broadcast captions by default.
    this.burnInSubtitles = options.burnInSubtitles ?? true;
    // Default to 'both' (stereo) for audio mode
    this.audioMode = options.audioMode ?? 'both';
    this.onTrack = options.onTrack;
    this.onConnectionStateChange = options.onConnectionStateChange;
    this.onSubtitle = options.onSubtitle;
    this.onEncodingRestarted = options.onEncodingRestarted;
    this.onError = options.onError;
    this.onLog = options.onLog;
  }

  private log(message: string): void {
    const timestamp = new Date().toLocaleTimeString();
    const logMsg = `[${timestamp}] ${message}`;
    console.log(`[RTCClient] ${message}`);
    this.onLog?.(logMsg);
  }

  async connect(): Promise<void> {
    this.log(`Connecting to channel ${this.channelId}`);
    this.onConnectionStateChange?.('connecting');

    try {
      // Create WebSocket connection
      await this.connectWebSocket();

      // Create PeerConnection
      this.createPeerConnection();

      // Request stream start (include burnInSubtitles and audioMode settings)
      this.sendMessage({ type: 'stream-start', channelId: this.channelId, burnInSubtitles: this.burnInSubtitles, audioMode: this.audioMode });

      // Start ping interval
      this.startPingInterval();
    } catch (error) {
      this.log(`Connection failed: ${error}`);
      this.onError?.(error as Error);
      this.onConnectionStateChange?.('failed');
      throw error;
    }
  }

  private connectWebSocket(): Promise<void> {
    return new Promise((resolve, reject) => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/api/ws/webrtc/${encodeURIComponent(this.channelId)}`;

      this.log(`Connecting WebSocket: ${wsUrl}`);
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        this.log('WebSocket connected');
        resolve();
      };

      this.ws.onerror = (event) => {
        this.log(`WebSocket error: ${event}`);
        reject(new Error('WebSocket connection failed'));
      };

      this.ws.onclose = (event) => {
        this.log(`WebSocket closed: ${event.code} ${event.reason}`);
        this.handleWebSocketClose();
      };

      this.ws.onmessage = (event) => {
        this.handleWebSocketMessage(event.data);
      };
    });
  }

  private createPeerConnection(): void {
    this.pc = new RTCPeerConnection({
      iceServers: [
        // No STUN/TURN for local network
      ],
    });

    // Handle incoming tracks
    this.pc.ontrack = (event) => {
      const track = event.track;
      this.log(`Received ${track.kind} track: id=${track.id}, enabled=${track.enabled}, muted=${track.muted}, readyState=${track.readyState}`);

      // Log track settings
      if (track.kind === 'video') {
        const settings = track.getSettings();
        this.log(`Video track settings: ${JSON.stringify(settings)}`);
      }

      // Always use a single combined MediaStream for all tracks
      // This prevents the issue where video and audio come in separate streams
      if (!this.mediaStream) {
        this.mediaStream = new MediaStream();
        this.log(`Created combined MediaStream: id=${this.mediaStream.id}`);
      }

      // Check if we already have a track of this kind
      const existingTracks = this.mediaStream.getTracks().filter(t => t.kind === track.kind);
      if (existingTracks.length > 0) {
        // Remove existing track of same kind
        existingTracks.forEach(t => {
          this.mediaStream!.removeTrack(t);
          this.log(`Removed existing ${t.kind} track`);
        });
      }

      // Add the new track to our combined stream
      this.mediaStream.addTrack(track);
      this.log(`Added ${track.kind} track to combined stream. Total tracks: video=${this.mediaStream.getVideoTracks().length}, audio=${this.mediaStream.getAudioTracks().length}`);

      // Only notify when we have at least a video track
      if (this.mediaStream.getVideoTracks().length > 0) {
        this.onTrack?.(track, this.mediaStream);
      }

      // Monitor track for unmute event
      track.onunmute = () => {
        this.log(`Track ${track.kind} unmuted`);
      };
      track.onmute = () => {
        this.log(`Track ${track.kind} muted`);
      };
      track.onended = () => {
        this.log(`Track ${track.kind} ended`);
      };
    };

    // Handle ICE candidates
    this.pc.onicecandidate = (event) => {
      if (event.candidate) {
        this.log(`Sending ICE candidate`);
        this.sendMessage({
          type: 'ice-candidate',
          peerId: this.peerId || undefined,
          candidate: event.candidate.toJSON(),
        });
      }
    };

    // Handle connection state changes
    this.pc.onconnectionstatechange = () => {
      const state = this.pc?.connectionState;
      this.log(`Connection state: ${state}`);

      switch (state) {
        case 'connected':
          this.onConnectionStateChange?.('connected');
          break;
        case 'disconnected':
          this.onConnectionStateChange?.('disconnected');
          break;
        case 'failed':
          this.onConnectionStateChange?.('failed');
          break;
        case 'closed':
          this.onConnectionStateChange?.('closed');
          break;
      }
    };

    // Handle data channel (for subtitles)
    this.pc.ondatachannel = (event) => {
      this.log(`Data channel received: ${event.channel.label}`);
      this.dataChannel = event.channel;
      this.dataChannel.binaryType = 'arraybuffer';

      this.dataChannel.onmessage = (msgEvent) => {
        try {
          // Handle both string and ArrayBuffer data
          let dataStr: string;
          if (msgEvent.data instanceof ArrayBuffer) {
            dataStr = new TextDecoder().decode(msgEvent.data);
          } else {
            dataStr = msgEvent.data;
          }
          const subtitle = JSON.parse(dataStr) as SubtitleMessage;
          this.onSubtitle?.(subtitle);
        } catch (e) {
          this.log(`Failed to parse subtitle: ${e}`);
        }
      };
    };

    this.log('PeerConnection created');
  }

  private async handleWebSocketMessage(data: string): Promise<void> {
    try {
      const msg: SignalingMessage = JSON.parse(data);

      switch (msg.type) {
        case 'offer':
          this.log('Received offer');
          this.peerId = msg.peerId || null;
          await this.handleOffer(msg.sdp!);
          break;

        case 'answer':
          this.log('Received answer');
          await this.handleAnswer(msg.sdp!);
          break;

        case 'ice-candidate':
          if (msg.candidate) {
            this.log('Received ICE candidate');
            await this.pc?.addIceCandidate(new RTCIceCandidate(msg.candidate));
          }
          break;

        case 'ready':
          this.log('Stream ready');
          break;

        case 'error':
          this.log(`Server error: ${msg.error}`);
          if (msg.requestId) this.pendingRestarts.delete(msg.requestId);
          this.onError?.(new Error(msg.error || 'Unknown error'), msg.requestId);
          break;

        case 'pong':
          // Ping response received
          break;

        case 'encoding-restarted':
          this.log(`Encoding restarted for channel ${msg.channelId}`);
          if (msg.requestId) {
            const pending = this.pendingRestarts.get(msg.requestId);
            if (pending) {
              this.channelId = pending.channelId;
              this.burnInSubtitles = pending.burnInSubtitles;
              this.audioMode = pending.audioMode;
              this.pendingRestarts.delete(msg.requestId);
            }
          } else if (msg.channelId) {
            this.channelId = msg.channelId;
          }
          this.onEncodingRestarted?.(msg.channelId || this.channelId, msg.requestId);
          break;
      }
    } catch (e) {
      this.log(`Failed to handle message: ${e}`);
    }
  }

  private async handleOffer(sdp: RTCSessionDescriptionInit): Promise<void> {
    if (!this.pc) return;

    await this.pc.setRemoteDescription(new RTCSessionDescription(sdp));

    const answer = await this.pc.createAnswer();
    await this.pc.setLocalDescription(answer);

    this.sendMessage({
      type: 'answer',
      peerId: this.peerId || undefined,
      sdp: answer,
    });

    this.log('Sent answer');
  }

  private async handleAnswer(sdp: RTCSessionDescriptionInit): Promise<void> {
    if (!this.pc) return;
    await this.pc.setRemoteDescription(new RTCSessionDescription(sdp));
    this.log('Set remote description');
  }

  private sendMessage(msg: SignalingMessage): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  private startPingInterval(): void {
    this.pingInterval = setInterval(() => {
      this.sendMessage({ type: 'ping' });
    }, 5000);
  }

  private handleWebSocketClose(): void {
    this.stopPingInterval();

    // Don't attempt reconnect - let the UI handle reconnection explicitly
    // Auto-reconnect causes race conditions and tuner leaks
    this.log('WebSocket closed - no auto-reconnect');
    this.onConnectionStateChange?.('closed');
  }

  private stopPingInterval(): void {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }

  async disconnect(): Promise<void> {
    this.log('Disconnecting');
    this._isDisconnecting = true;

    this.stopPingInterval();

    // Send stream-stop and wait for acknowledgment (with timeout)
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      const stopPromise = new Promise<void>((resolve) => {
        const timeout = setTimeout(() => {
          this.log('stream-stopped acknowledgment timeout, proceeding with close');
          resolve();
        }, 1000); // 1 second timeout

        const handler = (event: MessageEvent) => {
          try {
            const msg = JSON.parse(event.data);
            if (msg.type === 'stream-stopped') {
              this.log('Received stream-stopped acknowledgment');
              clearTimeout(timeout);
              resolve();
            }
          } catch {
            // Ignore parse errors
          }
        };
        this.ws!.addEventListener('message', handler, { once: true });
      });

      this.sendMessage({ type: 'stream-stop', peerId: this.peerId || undefined });
      await stopPromise;
    }

    // Close WebSocket
    if (this.ws) {
      this.ws.close(1000, 'Intentional disconnect');
      this.ws = null;
    }

    // Close PeerConnection
    if (this.pc) {
      this.pc.close();
      this.pc = null;
    }

    // Stop media stream
    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach((track) => track.stop());
      this.mediaStream = null;
    }

    this.peerId = null;
    this._isDisconnecting = false;
    this.onConnectionStateChange?.('closed');
  }

  get connectionState(): ConnectionStatus {
    if (!this.pc) return 'disconnected';

    switch (this.pc.connectionState) {
      case 'connected':
        return 'connected';
      case 'connecting':
      case 'new':
        return 'connecting';
      case 'disconnected':
        return 'disconnected';
      case 'failed':
        return 'failed';
      case 'closed':
        return 'closed';
      default:
        return 'disconnected';
    }
  }

  getMediaStream(): MediaStream | null {
    return this.mediaStream;
  }

  getPeerId(): string | null {
    return this.peerId;
  }

  /**
   * Restart encoding with new settings without reconnecting WebRTC.
   * This can be used for:
   * - Toggling ARIB caption display
   * - Changing channels
   * - Switching audio mode (dual mono)
   * @param options - New settings to apply
   */
  restartEncoding(options: { channelId?: string; burnInSubtitles?: boolean; audioMode?: AudioMode }): string {
    const newChannelId = options.channelId ?? this.channelId;
    const newBurnInSubtitles = options.burnInSubtitles ?? this.burnInSubtitles;
    const newAudioMode = options.audioMode ?? this.audioMode;
    const requestId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`;

    this.log(`Restarting encoding: channel=${newChannelId}, burnInSubtitles=${newBurnInSubtitles}, audioMode=${newAudioMode}`);
    this.pendingRestarts.set(requestId, { channelId: newChannelId, burnInSubtitles: newBurnInSubtitles, audioMode: newAudioMode });

    this.sendMessage({
      type: 'restart-encoding',
      requestId,
      channelId: newChannelId,
      burnInSubtitles: newBurnInSubtitles,
      audioMode: newAudioMode,
    });
    return requestId;
  }

  /**
   * Set audio mode for dual mono streams
   * @param mode - 'main' (left channel), 'sub' (right channel), or 'both' (stereo)
   */
  setAudioMode(mode: AudioMode): string {
    this.log(`Setting audio mode: ${mode}`);
    return this.restartEncoding({ audioMode: mode });
  }

  /**
   * Get current ARIB caption delivery setting
   */
  getBurnInSubtitles(): boolean {
    return this.burnInSubtitles;
  }

  /**
   * Get current audio mode
   */
  getAudioMode(): AudioMode {
    return this.audioMode;
  }
}

export type { SignalingMessage, SubtitleMessage, ConnectionStatus, RTCClientOptions, AudioMode };
