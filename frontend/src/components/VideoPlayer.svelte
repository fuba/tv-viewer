<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import { RTCClient, type ConnectionStatus, type AudioMode, type SubtitleMessage } from '../lib/webrtc/RTCClient'
  import { channelSelectionId } from '../lib/channelSelection'
  import { reconnectDelay, shouldRecoverStream } from '../lib/streamRecovery'
  import { createOrientationLockCoordinator, requestViewerFullscreen, type OrientationController } from '../lib/fullscreen'
  import { fittedFullscreenVideoSize, fittedVideoWidth } from '../lib/viewportFit'
  import { normalizedVolume } from '../lib/audioVolume'

  export let selectedChannel: any

  // Export logs for external display
  export let debugLogs: string[] = []
  export let pipelineLogs: string[] = []

  // Export connection status for MetaBar
  export let connectionStatus: ConnectionStatus = 'disconnected'
  export let streamStarted = false

  let videoElement: HTMLVideoElement
  let playerShell: HTMLDivElement
  let videoStage: HTMLDivElement
  let rtcClient: RTCClient | null = null
  let mediaStream: MediaStream | null = null
  let currentChannel: any = null
  let isStartingStream = false
  let audioEnabled = false
  let volume = 1
  let showVolumeControl = false
  let volumeButton: HTMLButtonElement
  let volumeSlider: HTMLInputElement
  let currentSessionId: string = ''
  let tracksReceived = 0
  let videoReadyState = 0
  let streamSequence = 0 // Used to ignore callbacks from old streams
  let videoWidth = 0 // Track video width for aspect ratio handling

  // Subtitle mode: true = display ARIB captions, false = hide captions
  let burnInSubtitles = true

  // Audio mode for dual mono streams (Japanese bilingual broadcasts)
  // 'both' = stereo, 'main' = left channel, 'sub' = right channel
  let audioMode: AudioMode = 'both'

  let activeSubtitles: Record<string, string> = {}
  let subtitleText = ''
  const subtitleTimers = new Map<string, ReturnType<typeof setTimeout>>()
  let pipelineLogTimer: ReturnType<typeof setTimeout> | null = null
  let recoveryTimer: ReturnType<typeof setTimeout> | null = null
  let recoveryStableTimer: ReturnType<typeof setTimeout> | null = null
  let recoveryAttempts = 0
  let destroyed = false
  let isFullscreen = false
  let fitToWindow = true
  let fittedWidth = 99_999
  let fitResizeObserver: ResizeObserver | null = null
  let fitAnimationFrame = 0
  let fullscreenStageWidth = 0
  let fullscreenStageHeight = 0
  let fullscreenControlsVisible = true
  let fullscreenControlsTimer: ReturnType<typeof setTimeout> | null = null
  const viewportSettleTimers = new Set<ReturnType<typeof setTimeout>>()
  let orientationLockCoordinator: ReturnType<typeof createOrientationLockCoordinator> | null = null
  let playerGesture: { pointerId: number, startedOutsideToolbar: boolean } | null = null
  let pendingRestart: {
    requestId: string
    previousChannel: any
    requestedChannel: any
    previousAudioMode: AudioMode
    requestedAudioMode: AudioMode
  } | null = null

  const maximumRecoveryAttempts = 5
  const recoveryStablePeriod = 120_000

  function orientationController(): OrientationController | undefined {
    return (screen as Screen & { orientation?: OrientationController }).orientation
  }

  function viewerOwnsFullscreen() {
    return document.fullscreenElement === playerShell
  }

  async function toggleFullscreen() {
    try {
      if (viewerOwnsFullscreen()) {
        await document.exitFullscreen()
        scheduleViewportSettle()
        return
      }
      if (document.fullscreenElement) return
      updateFullscreenStageSize()
      await requestViewerFullscreen(playerShell)
      scheduleViewportSettle()
    } catch (error) {
      addLog(`Fullscreen failed: ${error instanceof Error ? error.message : error}`, 'error')
    }
  }

  function handleFullscreenChange() {
    const wasFullscreen = isFullscreen
    isFullscreen = viewerOwnsFullscreen()
    if (isFullscreen) {
      revealFullscreenControls()
    } else if (wasFullscreen) {
      clearFullscreenControlsTimer()
      fullscreenControlsVisible = true
      showVolumeControl = false
    }
    orientationLockCoordinator?.setFullscreen(isFullscreen)
    scheduleViewportSettle()
  }

  function clearFullscreenControlsTimer() {
    if (!fullscreenControlsTimer) return
    clearTimeout(fullscreenControlsTimer)
    fullscreenControlsTimer = null
  }

  function revealFullscreenControls() {
    if (!isFullscreen) return
    fullscreenControlsVisible = true
    clearFullscreenControlsTimer()
    if (showVolumeControl) return
    fullscreenControlsTimer = setTimeout(() => {
      fullscreenControlsTimer = null
      fullscreenControlsVisible = false
    }, 3000)
  }

  function toggleFullscreenControls() {
    if (!isFullscreen) return
    if (fullscreenControlsVisible) {
      clearFullscreenControlsTimer()
      fullscreenControlsVisible = false
      return
    }
    revealFullscreenControls()
  }

  function eventIsInsideToolbar(event: PointerEvent) {
    return event.target instanceof Element && Boolean(event.target.closest('.player-toolbar'))
  }

  function handlePlayerPointerDown(event: PointerEvent) {
    if (!event.isPrimary || playerGesture) return
    playerGesture = {
      pointerId: event.pointerId,
      startedOutsideToolbar: !eventIsInsideToolbar(event),
    }
    try {
      playerShell.setPointerCapture(event.pointerId)
    } catch {
      // Pointer capture is optional for synthetic and interrupted gestures.
    }
  }

  function handlePlayerPointerUp(event: PointerEvent) {
    if (!playerGesture || playerGesture.pointerId !== event.pointerId) return
    const endElement = document.elementFromPoint(event.clientX, event.clientY)
    const endedInsidePlayer = endElement instanceof Element && playerShell.contains(endElement)
    const endedInsideToolbar = endElement instanceof Element && Boolean(endElement.closest('.player-toolbar'))
    const shouldHandle = playerGesture.startedOutsideToolbar && endedInsidePlayer && !endedInsideToolbar
    cancelPlayerGesture(event)
    if (!shouldHandle) return
    if (isFullscreen) {
      toggleFullscreenControls()
    } else {
      enableAudioFromUserGesture()
    }
  }

  function cancelPlayerGesture(event?: Event) {
    if (event instanceof PointerEvent && playerGesture && playerGesture.pointerId !== event.pointerId) return
    const gesture = playerGesture
    playerGesture = null
    if (gesture) {
      try {
        playerShell.releasePointerCapture(gesture.pointerId)
      } catch {
        // Capture may already be released after cancellation or focus loss.
      }
    }
  }

  function handleToolbarFocusIn() {
    if (!isFullscreen) return
    fullscreenControlsVisible = true
    clearFullscreenControlsTimer()
  }

  function handleToolbarFocusOut(event: FocusEvent) {
    const toolbar = event.currentTarget as HTMLElement
    if (event.relatedTarget instanceof Node && toolbar.contains(event.relatedTarget)) return
    revealFullscreenControls()
  }

  function clearViewportSettleTimers() {
    viewportSettleTimers.forEach(timer => clearTimeout(timer))
    viewportSettleTimers.clear()
  }

  function scheduleViewportSettle() {
    clearViewportSettleTimers()
    updateFittedWidth()
    for (const delay of [100, 300]) {
      const timer = setTimeout(() => {
        viewportSettleTimers.delete(timer)
        updateFittedWidth()
      }, delay)
      viewportSettleTimers.add(timer)
    }
  }

  function updateFullscreenStageSize() {
    const viewport = window.visualViewport
    const size = fittedFullscreenVideoSize(
      viewport?.width ?? window.innerWidth,
      viewport?.height ?? window.innerHeight,
    )
    fullscreenStageWidth = size.width
    fullscreenStageHeight = size.height
  }

  function updateFittedWidth() {
    cancelAnimationFrame(fitAnimationFrame)
    fitAnimationFrame = requestAnimationFrame(() => {
      if (!playerShell?.parentElement) return
      const viewport = window.visualViewport
      const viewportWidth = viewport?.width ?? window.innerWidth
      const viewportHeight = viewport?.height ?? window.innerHeight
      if (isFullscreen || viewerOwnsFullscreen()) {
        updateFullscreenStageSize()
        return
      }
      const topbar = document.querySelector<HTMLElement>('.topbar')
      const metaBar = document.querySelector<HTMLElement>('.meta-bar')
      const toolbar = playerShell.querySelector<HTMLElement>('.player-toolbar')
      const viewerMain = playerShell.closest<HTMLElement>('.viewer-main')
      const viewerStyle = viewerMain ? getComputedStyle(viewerMain) : null
      const viewerPadding = viewerStyle
        ? parseFloat(viewerStyle.paddingTop) + parseFloat(viewerStyle.paddingBottom)
        : 0
      const occupiedHeight = (topbar?.offsetHeight ?? 0) +
        (metaBar?.offsetHeight ?? 0) +
        (toolbar?.offsetHeight ?? 0) + viewerPadding
      const containerWidth = Math.min(
        playerShell.parentElement.clientWidth,
        viewportWidth,
      )
      fittedWidth = Math.round(fittedVideoWidth(
        containerWidth,
        viewportHeight,
        occupiedHeight,
      ))
    })
  }

  function toggleWindowFit() {
    fitToWindow = !fitToWindow
    if (fitToWindow) updateFittedWidth()
  }

  function updateVolume(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    volume = normalizedVolume(Number(input.value) / 100)
    audioEnabled = volume > 0
    if (videoElement) {
      videoElement.volume = volume
      videoElement.muted = !audioEnabled
      if (audioEnabled && videoElement.srcObject && videoElement.paused) {
        void videoElement.play().catch(() => {})
      }
    }
    try {
      localStorage.setItem('tv-viewer-volume', String(volume))
    } catch {
      // Playback must continue when persistence is unavailable.
    }
  }

  function syncMediaVolume() {
    if (!videoElement) return
    volume = normalizedVolume(videoElement.volume)
    audioEnabled = !videoElement.muted && volume > 0
    try {
      localStorage.setItem('tv-viewer-volume', String(volume))
    } catch {
      // Playback must continue when persistence is unavailable.
    }
  }

  async function toggleVolumeControl() {
    showVolumeControl = !showVolumeControl
    if (showVolumeControl) {
      clearFullscreenControlsTimer()
      await tick()
      volumeSlider?.focus()
    } else {
      revealFullscreenControls()
    }
  }

  function handleVolumeKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    showVolumeControl = false
    volumeButton?.focus()
  }

  onMount(() => {
    try {
      const storedVolume = localStorage.getItem('tv-viewer-volume')
      if (storedVolume !== null) volume = normalizedVolume(Number(storedVolume))
    } catch {
      volume = 1
    }
    if (videoElement) videoElement.volume = volume
    orientationLockCoordinator = createOrientationLockCoordinator(orientationController())
    document.addEventListener('fullscreenchange', handleFullscreenChange)
    fitResizeObserver = new ResizeObserver(updateFittedWidth)
    for (const element of [
      playerShell.parentElement,
      document.querySelector('.topbar'),
      document.querySelector('.meta-bar'),
      playerShell.querySelector('.player-toolbar'),
    ]) {
      if (element) fitResizeObserver.observe(element)
    }
    window.addEventListener('resize', updateFittedWidth)
    window.addEventListener('blur', cancelPlayerGesture)
    window.visualViewport?.addEventListener('resize', updateFittedWidth)
    orientationController()?.addEventListener?.('change', scheduleViewportSettle)
    updateFittedWidth()
  })

  function clearSubtitles() {
    subtitleTimers.forEach(timer => clearTimeout(timer))
    subtitleTimers.clear()
    activeSubtitles = {}
    subtitleText = ''
  }

  function updateSubtitleText() {
    subtitleText = Object.values(activeSubtitles).join('\n')
  }

  function handleSubtitle(message: SubtitleMessage) {
    if (!burnInSubtitles || message.type === 'clear') {
      clearSubtitles()
      return
    }
    const id = message.id || 'caption'
    if (message.type === 'hide') {
      const timer = subtitleTimers.get(id)
      if (timer) clearTimeout(timer)
      subtitleTimers.delete(id)
      delete activeSubtitles[id]
      activeSubtitles = { ...activeSubtitles }
      updateSubtitleText()
      return
    }
    if (message.type === 'show' && message.text) {
      activeSubtitles[id] = message.text
      activeSubtitles = { ...activeSubtitles }
      updateSubtitleText()
      const previous = subtitleTimers.get(id)
      if (previous) clearTimeout(previous)
      const durationSeconds = message.startTime !== undefined
        ? Math.max(0.1, (message.endTime ?? message.startTime + 5) - message.startTime)
        : Math.max(0.1, message.endTime ?? 5)
      subtitleTimers.set(id, setTimeout(() => {
        delete activeSubtitles[id]
        activeSubtitles = { ...activeSubtitles }
        subtitleTimers.delete(id)
        updateSubtitleText()
      }, Math.min(durationSeconds, 10) * 1000))
    }
  }

  // Toggle ARIB caption display.
  function toggleSubtitles() {
    burnInSubtitles = !burnInSubtitles
    if (!burnInSubtitles) clearSubtitles()
    addLog(`Subtitles: ${burnInSubtitles ? 'ON' : 'OFF'}`)
    // Captions are always received; toggling display does not interrupt media.
  }

  // Cycle through audio modes: both -> main -> sub -> both
  function cycleAudioMode() {
    const modes: AudioMode[] = ['both', 'main', 'sub']
    const currentIndex = modes.indexOf(audioMode)
    const requestedAudioMode = modes[(currentIndex + 1) % modes.length]
    addLog(`Audio mode: ${getAudioModeLabel(requestedAudioMode)}`)
    // Restart encoding with new setting if currently streaming
    if (rtcClient && streamStarted) {
      isStartingStream = true
      const requestId = rtcClient.setAudioMode(requestedAudioMode)
      pendingRestart = {
        requestId,
        previousChannel: currentChannel,
        requestedChannel: currentChannel,
        previousAudioMode: audioMode,
        requestedAudioMode,
      }
    } else {
      audioMode = requestedAudioMode
    }
  }

  // Get display label for audio mode
  function getAudioModeLabel(mode: AudioMode): string {
    switch (mode) {
      case 'main': return '主'
      case 'sub': return '副'
      case 'both': return '主+副'
    }
  }

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

  function cancelRecovery() {
    if (recoveryTimer) {
      clearTimeout(recoveryTimer)
      recoveryTimer = null
    }
  }

  function cancelRecoveryStability() {
    if (recoveryStableTimer) {
      clearTimeout(recoveryStableTimer)
      recoveryStableTimer = null
    }
  }

  function markRecoveryStable(sequence: number) {
    cancelRecoveryStability()
    recoveryStableTimer = setTimeout(() => {
      recoveryStableTimer = null
      if (!destroyed && sequence === streamSequence && connectionStatus === 'connected') {
        recoveryAttempts = 0
      }
    }, recoveryStablePeriod)
  }

  function scheduleRecovery(channel: any, sequence: number, state: ConnectionStatus) {
    if (!shouldRecoverStream(state, destroyed, Boolean(channel)) || sequence !== streamSequence || recoveryTimer) {
      return
    }

    cancelRecoveryStability()
    if (recoveryAttempts >= maximumRecoveryAttempts) {
      addLog(`Automatic stream recovery stopped after ${maximumRecoveryAttempts} attempts`, 'error')
      return
    }

    recoveryAttempts++
    const delay = reconnectDelay(recoveryAttempts)
    addLog(`Stream ended unexpectedly; reconnecting in ${delay / 1000}s (attempt ${recoveryAttempts}/${maximumRecoveryAttempts})`, 'error')
    recoveryTimer = setTimeout(() => {
      recoveryTimer = null
      if (destroyed || sequence !== streamSequence || currentChannel !== channel) return
      isStartingStream = false
      void startStream(channel, true)
    }, delay)
  }

  async function startStream(channel: any, recovering = false) {
    cancelRecoveryStability()
    if (!recovering) {
      recoveryAttempts = 0
      cancelRecovery()
    }
    if (isStartingStream) {
      addLog('Stream already starting, forcing restart for new channel')
    }

    isStartingStream = true
    const startTime = Date.now()
    tracksReceived = 0
    videoWidth = 0

    const channelId = channelSelectionId(channel)

    addLog(`[${startTime}] Starting WebRTC stream for channel ${channelId} (${channel.name})`)

    // If we already have an active connection, use restartEncoding to switch channels
    if (rtcClient && connectionStatus === 'connected' && currentChannel) {
      addLog(`Switching channel from ${currentChannel.channel} to ${channelId} using restart-encoding`)
      clearSubtitles()
      const requestId = rtcClient.restartEncoding({ channelId, burnInSubtitles: true, audioMode })
      pendingRestart = {
        requestId,
        previousChannel: currentChannel,
        requestedChannel: channel,
        previousAudioMode: audioMode,
        requestedAudioMode: audioMode,
      }
      return
    }

    // Increment only when replacing the RTCClient so its callbacks remain valid across restarts.
    streamSequence++
    const thisStreamSequence = streamSequence

    // No active connection - create a new one
    currentChannel = channel
    streamStarted = false

    // Cleanup existing connection if any
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

    try {
      // Create WebRTC client with the current subtitle setting.
      rtcClient = new RTCClient({
        channelId,
        burnInSubtitles: true,
        audioMode,
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
              videoElement.volume = volume
              videoElement.muted = !audioEnabled || volume === 0
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
          markRecoveryStable(thisStreamSequence)
        },
        onConnectionStateChange: (state) => {
          if (thisStreamSequence !== streamSequence || destroyed) return
          connectionStatus = state
          addLog(`[WebRTC] Connection state: ${state}`)

          if (state === 'connected') {
            addLog('WebRTC connected - low latency streaming active', 'success')
          } else if (state === 'failed') {
            addLog('WebRTC connection failed', 'error')
            isStartingStream = false
          }
          scheduleRecovery(currentChannel, thisStreamSequence, state)
        },
        onError: (error, requestId) => {
          if (thisStreamSequence !== streamSequence || destroyed) return
          addLog(`WebRTC error: ${error.message}`, 'error')
          if (requestId && pendingRestart?.requestId === requestId) {
            selectedChannel = pendingRestart.previousChannel
            currentChannel = pendingRestart.previousChannel
            audioMode = pendingRestart.previousAudioMode
            pendingRestart = null
          }
          isStartingStream = false
        },
        onEncodingRestarted: (newChannelId, requestId) => {
          if (thisStreamSequence !== streamSequence || destroyed) return
          if (requestId && pendingRestart?.requestId === requestId) {
            currentChannel = pendingRestart.requestedChannel
            selectedChannel = pendingRestart.requestedChannel
            audioMode = pendingRestart.requestedAudioMode
            pendingRestart = null
          }
          addLog(`Encoding restarted for channel ${newChannelId}`, 'success')
          isStartingStream = false
          schedulePipelineLogs(newChannelId)
        },
        onSubtitle: handleSubtitle,
        onLog: (message) => {
          debugLogs = [message, ...debugLogs.slice(0, 99999)]
        }
      })

      // Connect
      await rtcClient.connect()
      currentSessionId = rtcClient.getPeerId() || channelId

      schedulePipelineLogs(channelId)

    } catch (error) {
      addLog(`Failed to start WebRTC stream: ${error}`, 'error')
      isStartingStream = false
    }
  }

  async function fetchPipelineLogs(channelId: string) {
    try {
      const response = await fetch(`/api/logs/${encodeURIComponent(channelId)}`)
      if (response.ok) {
        const data = await response.json()
        pipelineLogs = data.logs || []
      }
    } catch (error) {
      // Silently fail for log fetching
    }

  }

  function schedulePipelineLogs(channelId: string) {
    if (pipelineLogTimer) clearTimeout(pipelineLogTimer)
    pipelineLogTimer = setTimeout(async () => {
      await fetchPipelineLogs(channelId)
      if (streamStarted && rtcClient) schedulePipelineLogs(channelId)
    }, 2000)
  }

  onDestroy(() => {
    destroyed = true
    cancelRecovery()
    cancelRecoveryStability()
    document.removeEventListener('fullscreenchange', handleFullscreenChange)
    window.removeEventListener('resize', updateFittedWidth)
    window.removeEventListener('blur', cancelPlayerGesture)
    window.visualViewport?.removeEventListener('resize', updateFittedWidth)
    orientationController()?.removeEventListener?.('change', scheduleViewportSettle)
    fitResizeObserver?.disconnect()
    cancelAnimationFrame(fitAnimationFrame)
    clearFullscreenControlsTimer()
    clearViewportSettleTimers()
    orientationLockCoordinator?.destroy()
    orientationLockCoordinator = null
    clearSubtitles()
    if (pipelineLogTimer) clearTimeout(pipelineLogTimer)
    if (rtcClient) {
      rtcClient.disconnect()
      rtcClient = null
    }

    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop())
      mediaStream = null
    }
  })

  export function enableAudioFromUserGesture() {
    audioEnabled = volume > 0
    if (videoElement) {
      videoElement.volume = volume
      videoElement.muted = !audioEnabled
      if (videoElement.srcObject && videoElement.paused) {
        void videoElement.play().catch((error) => {
          addLog(`Audio activation play failed: ${error.message}`, 'error')
        })
      }
    }
    addLog('Audio enabled by channel selection', 'success')
  }

</script>

<!-- Theater mode: full width video container -->
<div
  class="player-shell"
  class:fit-window={fitToWindow}
  class:fullscreen-controls-hidden={isFullscreen && !fullscreenControlsVisible}
  class:volume-control-open={isFullscreen && showVolumeControl}
  style="--fit-width: {fittedWidth}px; --fullscreen-stage-width: {fullscreenStageWidth}px; --fullscreen-stage-height: {fullscreenStageHeight}px"
  bind:this={playerShell}
  on:pointerdown={handlePlayerPointerDown}
  on:pointerup={handlePlayerPointerUp}
  on:pointercancel={cancelPlayerGesture}
  on:lostpointercapture={cancelPlayerGesture}
>
  <div class="video-stage" bind:this={videoStage}>
    {#if selectedChannel}
      <!-- svelte-ignore a11y-media-has-caption -->
      <video
        bind:this={videoElement}
        class="w-full h-full"
        style:object-fit={videoWidth === 1440 ? 'fill' : 'contain'}
        controls={!isFullscreen}
        autoplay
        playsinline
        muted={!audioEnabled}
        on:volumechange={syncMediaVolume}
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

      {#if burnInSubtitles && subtitleText}
        <div class="live-subtitle-layer" aria-live="polite">
          <div class="live-subtitle-body">
            {subtitleText}
          </div>
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

  <div
    class="player-toolbar"
    aria-label="再生設定"
    on:pointerdown={revealFullscreenControls}
    on:pointerup={revealFullscreenControls}
    on:focusin={handleToolbarFocusIn}
    on:focusout={handleToolbarFocusOut}
  >
    <div class="toolbar-channel">
      <span class="toolbar-status" class:connected={connectionStatus === 'connected'}></span>
      <span>{currentChannel?.displayName || currentChannel?.name || selectedChannel?.displayName || selectedChannel?.name || 'チャンネル未選択'}</span>
    </div>
    <div class="toolbar-actions">
      <button class="toolbar-button" on:click={cycleAudioMode} disabled={isStartingStream}>
        <span class="toolbar-label">音声</span>
        <strong>{getAudioModeLabel(audioMode)}</strong>
      </button>
      <button class="toolbar-button" class:active={burnInSubtitles} on:click={toggleSubtitles} disabled={isStartingStream} aria-pressed={burnInSubtitles}>
        <span class="toolbar-label">字幕</span>
        <strong>{burnInSubtitles ? '入' : '切'}</strong>
      </button>
      <div class="volume-control">
        <button
          class="toolbar-button volume-button"
          class:active={showVolumeControl}
          bind:this={volumeButton}
          on:click={toggleVolumeControl}
          aria-label="音量を調整"
          aria-expanded={showVolumeControl}
          aria-controls="volume-control-popover"
        >
          <svg viewBox="0 0 24 24" aria-hidden="true">
            {#if !audioEnabled || volume === 0}
              <path d="M11 5 6.5 9H3v6h3.5l4.5 4V5ZM16 9l5 6M21 9l-5 6" />
            {:else}
              <path d="M11 5 6.5 9H3v6h3.5l4.5 4V5ZM15 9.5a4 4 0 0 1 0 5M18 7a7 7 0 0 1 0 10" />
            {/if}
          </svg>
          <span>{audioEnabled ? `${Math.round(volume * 100)}%` : '消音'}</span>
        </button>
        {#if showVolumeControl}
          <div class="volume-popover" id="volume-control-popover">
            <input
              bind:this={volumeSlider}
              id="volume-slider"
              type="range"
              min="0"
              max="100"
              step="1"
              value={Math.round(volume * 100)}
              on:input={updateVolume}
              on:keydown={handleVolumeKeydown}
              aria-label="音量"
            />
            <output for="volume-slider">{Math.round(volume * 100)}%</output>
          </div>
        {/if}
      </div>
      <button
        class="toolbar-button fit-button"
        class:active={fitToWindow}
        on:click={toggleWindowFit}
        aria-label={fitToWindow ? '通常サイズで表示' : 'ブラウザに合わせて表示'}
        aria-pressed={fitToWindow}
        title={fitToWindow ? '通常サイズに戻す' : 'ブラウザに合わせる'}
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="3" y="5" width="18" height="14" rx="2" />
          <path d="M8 9H6v2M16 9h2v2M8 15H6v-2M16 15h2v-2" />
        </svg>
        <span>画面に合わせる</span>
      </button>
      <button class="toolbar-button fullscreen-button" on:click={toggleFullscreen} aria-label={isFullscreen ? '全画面を終了' : '全画面で見る'}>
        <svg viewBox="0 0 24 24" aria-hidden="true">
          {#if isFullscreen}
            <path d="M9 3v6H3M15 3v6h6M9 21v-6H3M15 21v-6h6" />
          {:else}
            <path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5" />
          {/if}
        </svg>
        <span>全画面</span>
      </button>
    </div>
  </div>
</div>
