<script lang="ts">
  import { onDestroy } from 'svelte'
  import { RTCClient, type ConnectionStatus, type SubtitleMessage } from '../lib/webrtc/RTCClient'

  export let selectedChannel: any

  // Export logs for external display
  export let debugLogs: string[] = []
  export let ffmpegLogs: string[] = []

  // Export connection status for MetaBar
  export let connectionStatus: ConnectionStatus = 'disconnected'
  export let streamStarted = false

  let videoElement: HTMLVideoElement
  let rtcClient: RTCClient | null = null
  let mediaStream: MediaStream | null = null
  let currentChannel: any = null
  let isStartingStream = false
  let hasUserInteracted = false
  let currentSessionId: string = ''
  let subtitles: SubtitleMessage[] = []
  let tracksReceived = 0
  let videoReadyState = 0
  let streamSequence = 0 // Used to ignore callbacks from old streams
  let videoWidth = 0 // Track video width for aspect ratio handling

  function addLog(message: string, type: 'info' | 'error' | 'success' = 'info') {
    const timestamp = new Date().toLocaleTimeString()
    const logEntry = `[${timestamp}] ${message}`
    debugLogs = [logEntry, ...debugLogs.slice(0, 99999)]
    console.log(message)
  }

  $: if (selectedChannel && videoElement) {
    if (!isStartingStream && selectedChannel !== currentChannel) {
      startStream(selectedChannel)
    }
  }

  async function startStream(channel: any) {
    // Increment sequence to invalidate any pending callbacks from old streams
    streamSequence++
    const thisStreamSequence = streamSequence

    if (isStartingStream) {
      addLog('Stream already starting, forcing restart for new channel')
    }

    isStartingStream = true
    currentChannel = channel
    const startTime = Date.now()
    streamStarted = false
    tracksReceived = 0

    addLog(`[${startTime}] Starting WebRTC stream for channel ${channel.channel} (${channel.name}) [seq=${thisStreamSequence}]`)

    // Cleanup existing connection - wait for disconnect to complete
    if (rtcClient) {
      await rtcClient.disconnect()
      rtcClient = null
    }

    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop())
      mediaStream = null
    }

    if (videoElement) {
      videoElement.srcObject = null
    }

    connectionStatus = 'disconnected'
    subtitles = []
    videoWidth = 0

    const channelId = channel.channel || channel.id

    try {
      // Create WebRTC client
      rtcClient = new RTCClient({
        channelId,
        onTrack: (track, stream) => {
          // Ignore callbacks from old streams
          if (thisStreamSequence !== streamSequence) {
            addLog(`[${Date.now() - startTime}ms] Ignoring track from old stream [seq=${thisStreamSequence}, current=${streamSequence}]`)
            return
          }

          tracksReceived++
          addLog(`[${Date.now() - startTime}ms] Received ${track.kind} track (total: ${tracksReceived})`, 'success')
          addLog(`[${Date.now() - startTime}ms] Stream tracks: video=${stream.getVideoTracks().length}, audio=${stream.getAudioTracks().length}`)

          // Only set srcObject once, when video track is first available
          const needsSourceUpdate = !mediaStream || mediaStream !== stream
          mediaStream = stream

          if (videoElement && needsSourceUpdate && stream.getVideoTracks().length > 0) {
            // Only set srcObject if it's different from current
            if (videoElement.srcObject !== stream) {
              videoElement.srcObject = stream
              videoReadyState = videoElement.readyState
              addLog(`[${Date.now() - startTime}ms] Video source set, readyState=${videoReadyState}`, 'success')

              // iOS requires explicit play() call after setting srcObject
              videoElement.play().then(() => {
                if (thisStreamSequence !== streamSequence) return // Check again after async
                videoReadyState = videoElement.readyState
                addLog(`[${Date.now() - startTime}ms] Video playback started, readyState=${videoReadyState}`, 'success')
              }).catch((err) => {
                if (thisStreamSequence !== streamSequence) return
                addLog(`[${Date.now() - startTime}ms] Video play error: ${err.message}`, 'error')
              })
            } else {
              addLog(`[${Date.now() - startTime}ms] Stream already set, skipping`, 'info')
            }
          } else if (!videoElement) {
            addLog(`[${Date.now() - startTime}ms] WARNING: videoElement is null!`, 'error')
          }

          streamStarted = true
          isStartingStream = false
        },
        onConnectionStateChange: (state) => {
          connectionStatus = state
          addLog(`[WebRTC] Connection state: ${state}`)

          if (state === 'connected') {
            addLog('WebRTC connected - low latency streaming active', 'success')
          } else if (state === 'failed') {
            addLog('WebRTC connection failed', 'error')
            isStartingStream = false
          }
        },
        onSubtitle: (subtitle) => {
          if (subtitle.type === 'show') {
            subtitles = [...subtitles, subtitle]
          } else if (subtitle.type === 'hide' || subtitle.type === 'clear') {
            subtitles = subtitles.filter(s => s.id !== subtitle.id)
          }
        },
        onError: (error) => {
          addLog(`WebRTC error: ${error.message}`, 'error')
          isStartingStream = false
        },
        onLog: (message) => {
          debugLogs = [message, ...debugLogs.slice(0, 99999)]
        }
      })

      // Connect
      await rtcClient.connect()
      currentSessionId = rtcClient.getPeerId() || channelId

      // Fetch FFmpeg logs periodically
      fetchFFmpegLogs(channelId)

    } catch (error) {
      addLog(`Failed to start WebRTC stream: ${error}`, 'error')
      isStartingStream = false
    }
  }

  async function fetchFFmpegLogs(channelId: string) {
    try {
      const response = await fetch(`/api/logs/${encodeURIComponent(channelId)}`)
      if (response.ok) {
        const data = await response.json()
        ffmpegLogs = data.logs || []
      }
    } catch (error) {
      // Silently fail for log fetching
    }

    // Continue fetching logs every 2 seconds while streaming
    if (streamStarted && rtcClient) {
      setTimeout(() => fetchFFmpegLogs(channelId), 2000)
    }
  }

  onDestroy(() => {
    if (rtcClient) {
      rtcClient.disconnect()
      rtcClient = null
    }

    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop())
      mediaStream = null
    }
  })

  function handleVideoClick() {
    if (videoElement && !hasUserInteracted) {
      hasUserInteracted = true
      videoElement.muted = false
      addLog('Audio enabled after user interaction')
    }
  }
</script>

<!-- Theater mode: full width video container -->
<div class="w-full h-full flex items-center justify-center">
  <div class="w-full max-w-[1800px] aspect-video bg-black relative">
    {#if selectedChannel}
      <!-- svelte-ignore a11y-media-has-caption -->
      <video
        bind:this={videoElement}
        class="w-full h-full"
        style="object-fit: {videoWidth === 1440 ? 'fill' : 'contain'};"
        controls
        autoplay
        playsinline
        muted
        on:click={handleVideoClick}
        on:play={() => {
          addLog(`Video play event, paused=${videoElement?.paused}, currentTime=${videoElement?.currentTime}`)
        }}
        on:playing={() => {
          addLog(`Video playing event`)
        }}
        on:waiting={() => {
          addLog(`Video waiting event (buffering)`)
        }}
        on:stalled={() => {
          addLog(`Video stalled event`)
        }}
        on:error={() => {
          const err = videoElement?.error
          addLog(`Video error: ${err?.code} ${err?.message}`, 'error')
        }}
        on:loadeddata={() => {
          videoWidth = videoElement?.videoWidth || 0
          addLog(`Video loadeddata, videoWidth=${videoWidth}, videoHeight=${videoElement?.videoHeight}`)
        }}
      >
      </video>

      <!-- Subtitle overlay -->
      {#if subtitles.length > 0}
        <div class="absolute bottom-16 left-0 right-0 text-center pointer-events-none">
          {#each subtitles as subtitle}
            <div class="inline-block bg-black/70 text-white px-3 py-1 rounded text-lg">
              {subtitle.text}
            </div>
          {/each}
        </div>
      {/if}

      <!-- Loading indicator -->
      {#if isStartingStream}
        <div class="absolute inset-0 flex items-center justify-center bg-black/50">
          <div class="text-white text-center">
            <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-white mx-auto mb-4"></div>
            <p>WebRTC接続中...</p>
          </div>
        </div>
      {/if}
    {:else}
      <div class="flex items-center justify-center h-full text-gray-500">
        <p class="text-xl">チャンネルを選択してください</p>
      </div>
    {/if}
  </div>
</div>
