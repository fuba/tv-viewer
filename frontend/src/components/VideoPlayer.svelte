<script lang="ts">
  import { createEventDispatcher, onDestroy, onMount, tick } from 'svelte'
  import { RTCClient, type ConnectionStatus, type AudioMode, type SubtitleMessage, type TranslationMessage } from '../lib/webrtc/RTCClient'
  import { channelSelectionId } from '../lib/channelSelection'
  import { reconnectDelay, shouldRecoverStream } from '../lib/streamRecovery'
  import { createOrientationLockCoordinator, requestViewerFullscreen, type OrientationController } from '../lib/fullscreen'
  import { fittedFullscreenVideoSize } from '../lib/viewportFit'
  import { normalizedVolume } from '../lib/audioVolume'
  import { broadcastDisplayAspect, displayAspectRatio, type VideoFormatMessage } from '../lib/videoFormat'
  import {
    clearTranslationDraft,
    emptyTranslationHistories,
    emptyTranslationLog,
    pruneTranslationHistories,
    translationHistoryState,
    translationMessageRoute,
    translationStatusAfterRestart,
    translationStatusClearsDraft,
    type TranslationHistories,
    type TranslationLogState,
    type TranslationUIStatus,
  } from '../lib/translationLog'
  import TunerStatus from './TunerStatus.svelte'
  import SubtitleRenderer from './SubtitleRenderer.svelte'
  import TranslationLogOverlay from './TranslationLogOverlay.svelte'

  const dispatch = createEventDispatcher()

  export let selectedChannel: any

  // Export logs for external display
  export let debugLogs: string[] = []
  export let pipelineLogs: string[] = []

  // Exposed so the shell can react to the connection state
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
  // Intrinsic size of the decoded picture, used to shape the video frame.
  let videoWidth = 0
  let videoHeight = 0

  // Subtitle mode: true = display ARIB captions, false = hide captions
  let burnInSubtitles = true

  // Audio mode for dual mono streams (Japanese bilingual broadcasts)
  // 'both' = stereo, 'main' = left channel, 'sub' = right channel
  let audioMode: AudioMode = 'both'

  // VoiceTranslate replaces broadcast audio and renders a separate translation log.
  let translationEnabled = false
  let translationStatus: TranslationUIStatus = 'off'
  let translationHistories: TranslationHistories = emptyTranslationHistories()
  let translationChannelId = ''
  let activeTranslationLog: TranslationLogState = emptyTranslationLog()
  let activeTranslationStreamId = ''
  const retiredTranslationStreamIds = new Set<string>()
  let translationPruneTimer: ReturnType<typeof setInterval> | null = null
  let lastTranslationErrorAt = 0

  $: translationChannelId = channelSelectionId(currentChannel || selectedChannel)
  $: activeTranslationLog = translationHistories[translationChannelId] ?? emptyTranslationLog()

  let activeSubtitles: Record<string, SubtitleMessage> = {}
  let activeCaptions: SubtitleMessage[] = []
  const subtitleTimers = new Map<string, ReturnType<typeof setTimeout>>()
  let pipelineLogTimer: ReturnType<typeof setTimeout> | null = null
  let startWatchdogTimer: ReturnType<typeof setTimeout> | null = null
  let startWatchdogReleases = 0
  let recoveryTimer: ReturnType<typeof setTimeout> | null = null
  let recoveryStableTimer: ReturnType<typeof setTimeout> | null = null
  let recoveryAttempts = 0
  let destroyed = false
  let isFullscreen = false
  let frameResizeObserver: ResizeObserver | null = null
  let frameAnimationFrame = 0
  let frameWidth = 0
  let frameHeight = 0
  let controlsVisible = true
  let controlsTimer: ReturnType<typeof setTimeout> | null = null
  let barHovered = false
  const viewportSettleTimers = new Set<ReturnType<typeof setTimeout>>()
  let orientationLockCoordinator: ReturnType<typeof createOrientationLockCoordinator> | null = null
  let playerGesture: { pointerId: number } | null = null
  let pendingRestart: {
    requestId: string
    previousChannel: any
    requestedChannel: any
    previousAudioMode: AudioMode
    requestedAudioMode: AudioMode
    previousTranslationEnabled: boolean
    requestedTranslationEnabled: boolean
    previousTranslationStatus: TranslationUIStatus
    previousTranslationStreamId: string
  } | null = null

  const maximumRecoveryAttempts = 5
  const recoveryStablePeriod = 120_000
  const startWatchdogPeriod = 12_000
  const maximumStartWatchdogReleases = 3

  function orientationController(): OrientationController | undefined {
    return (screen as Screen & { orientation?: OrientationController }).orientation
  }

  // iOS Safari cannot fullscreen a container element; it only exposes the native
  // video player, which rotates itself to match the picture on a phone.
  type NativeFullscreenVideo = HTMLVideoElement & {
    webkitEnterFullscreen?: () => void
    webkitExitFullscreen?: () => void
    webkitDisplayingFullscreen?: boolean
  }

  function nativeFullscreenVideo(): NativeFullscreenVideo | null {
    const video = videoElement as NativeFullscreenVideo | undefined
    return video?.webkitEnterFullscreen ? video : null
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
      const nativeVideo = nativeFullscreenVideo()
      if (nativeVideo?.webkitDisplayingFullscreen) {
        nativeVideo.webkitExitFullscreen?.()
        return
      }
      if (document.fullscreenElement) return
      if (typeof playerShell.requestFullscreen === 'function') {
        await requestViewerFullscreen(playerShell)
        scheduleViewportSettle()
        return
      }
      if (nativeVideo) {
        nativeVideo.webkitEnterFullscreen?.()
        return
      }
      addLog('Fullscreen is not supported by this browser', 'error')
    } catch (error) {
      addLog(`Fullscreen failed: ${error instanceof Error ? error.message : error}`, 'error')
    }
  }

  function handleFullscreenChange() {
    isFullscreen = viewerOwnsFullscreen()
    revealControls()
    orientationLockCoordinator?.setFullscreen(isFullscreen)
    scheduleViewportSettle()
  }

  function handleNativeFullscreenEnter() {
    isFullscreen = true
    orientationLockCoordinator?.setFullscreen(true)
  }

  function handleNativeFullscreenExit() {
    isFullscreen = false
    orientationLockCoordinator?.setFullscreen(false)
    revealControls()
    scheduleViewportSettle()
  }

  // Svelte's DOM typings do not know the iOS-only fullscreen events.
  function nativeFullscreenEvents(node: HTMLVideoElement) {
    node.addEventListener('webkitbeginfullscreen', handleNativeFullscreenEnter)
    node.addEventListener('webkitendfullscreen', handleNativeFullscreenExit)
    return {
      destroy() {
        node.removeEventListener('webkitbeginfullscreen', handleNativeFullscreenEnter)
        node.removeEventListener('webkitendfullscreen', handleNativeFullscreenExit)
      },
    }
  }

  function clearControlsTimer() {
    if (!controlsTimer) return
    clearTimeout(controlsTimer)
    controlsTimer = null
  }

  // The bar overlays the picture, so it fades out unless something needs it on screen.
  function revealControls() {
    controlsVisible = true
    clearControlsTimer()
    if (showVolumeControl || barHovered || !selectedChannel) return
    controlsTimer = setTimeout(() => {
      controlsTimer = null
      controlsVisible = false
    }, 2600)
  }

  function toggleControls() {
    if (controlsVisible) {
      clearControlsTimer()
      controlsVisible = false
      return
    }
    revealControls()
  }

  function handleBarPointerEnter() {
    barHovered = true
    revealControls()
  }

  function handleBarPointerLeave() {
    barHovered = false
    revealControls()
  }

  function elementIsInsidePlayerUI(element: Element) {
    return Boolean(element.closest('.player-bar, .translation-log-overlay'))
  }

  function eventIsInsidePlayerUI(event: PointerEvent) {
    return event.target instanceof Element && elementIsInsidePlayerUI(event.target)
  }

  function handlePlayerPointerDown(event: PointerEvent) {
    if (!event.isPrimary || playerGesture) return
    // Never capture a gesture that starts on interactive player UI: capturing
    // retargets the follow-up click and would break buttons and log scrolling.
    if (eventIsInsidePlayerUI(event)) return
    playerGesture = { pointerId: event.pointerId }
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
    const endedInsidePlayerUI = endElement instanceof Element && elementIsInsidePlayerUI(endElement)
    cancelPlayerGesture(event)
    // Both press and release must happen on the picture, never on player UI.
    if (!endedInsidePlayer || endedInsidePlayerUI) return
    enableAudioFromUserGesture()
    toggleControls()
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
    controlsVisible = true
    clearControlsTimer()
  }

  function handleToolbarFocusOut(event: FocusEvent) {
    const toolbar = event.currentTarget as HTMLElement
    if (event.relatedTarget instanceof Node && toolbar.contains(event.relatedTarget)) return
    revealControls()
  }

  function clearViewportSettleTimers() {
    viewportSettleTimers.forEach(timer => clearTimeout(timer))
    viewportSettleTimers.clear()
  }

  function clearStartWatchdog() {
    if (!startWatchdogTimer) return
    clearTimeout(startWatchdogTimer)
    startWatchdogTimer = null
  }

  // A switch that never reports back would otherwise leave isStartingStream latched
  // and every later channel change silently ignored.
  function beginStartAttempt() {
    isStartingStream = true
    clearStartWatchdog()
    if (startWatchdogReleases >= maximumStartWatchdogReleases) return
    startWatchdogTimer = setTimeout(() => {
      startWatchdogTimer = null
      if (destroyed || !isStartingStream) return
      startWatchdogReleases++
      pendingRestart = null
      addLog(`Channel switch did not complete in ${startWatchdogPeriod / 1000}s; releasing the switch lock`, 'error')
      isStartingStream = false
    }, startWatchdogPeriod)
  }

  function settleStartAttempt() {
    isStartingStream = false
    clearStartWatchdog()
  }

  function scheduleViewportSettle() {
    clearViewportSettleTimers()
    updateFrameSize()
    for (const delay of [100, 300]) {
      const timer = setTimeout(() => {
        viewportSettleTimers.delete(timer)
        updateFrameSize()
      }, delay)
      viewportSettleTimers.add(timer)
    }
  }

  // The server parses the MPEG-2 sequence header and announces the display aspect,
  // because the coded size the browser reports never means square pixels: 1440x1080
  // and 720x480 are both 16:9 broadcasts. 16:9 stands in until the first announcement.
  let displayAspect = broadcastDisplayAspect

  function applyVideoFormat(format: VideoFormatMessage) {
    const aspect = displayAspectRatio(format)
    if (aspect === displayAspect) return
    displayAspect = aspect
    addLog(`Source ${format.width}x${format.height} displayed at ${format.aspectNum}:${format.aspectDen}`)
    updateFrameSize()
  }

  // Inscribe the picture frame in the stage so the video always touches two viewport
  // edges, and keep it identical on every channel.
  function updateFrameSize() {
    cancelAnimationFrame(frameAnimationFrame)
    frameAnimationFrame = requestAnimationFrame(() => {
      if (!videoStage) return
      const size = fittedFullscreenVideoSize(videoStage.clientWidth, videoStage.clientHeight, displayAspect)
      frameWidth = Math.round(size.width)
      frameHeight = Math.round(size.height)
    })
  }

  function logIntrinsicSize() {
    if (!videoElement) return
    const width = videoElement.videoWidth || 0
    const height = videoElement.videoHeight || 0
    if (width === videoWidth && height === videoHeight) return
    videoWidth = width
    videoHeight = height
    addLog(`Video size: ${width}x${height} (displayed as 16:9)`)
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
      clearControlsTimer()
      await tick()
      volumeSlider?.focus()
    } else {
      revealControls()
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
    frameResizeObserver = new ResizeObserver(updateFrameSize)
    if (videoStage) frameResizeObserver.observe(videoStage)
    window.addEventListener('resize', updateFrameSize)
    window.addEventListener('blur', cancelPlayerGesture)
    window.visualViewport?.addEventListener('resize', updateFrameSize)
    orientationController()?.addEventListener?.('change', scheduleViewportSettle)
    translationPruneTimer = setInterval(() => {
      translationHistories = pruneTranslationHistories(translationHistories, Date.now())
    }, 60_000)
    updateFrameSize()
    revealControls()
  })

  function clearSubtitles() {
    subtitleTimers.forEach(timer => clearTimeout(timer))
    subtitleTimers.clear()
    activeSubtitles = {}
    activeCaptions = []
  }

  function clearCurrentTranslationDraft() {
    translationHistories = clearTranslationDraft(translationHistories, translationChannelId)
  }

  function requestedTranslationEnabled(): boolean {
    return pendingRestart?.requestedTranslationEnabled ?? translationEnabled
  }

  function translationMessageChannelId(): string {
    return channelSelectionId(pendingRestart?.requestedChannel || currentChannel || selectedChannel)
  }

  function retireActiveTranslationStream(): string {
    const previous = activeTranslationStreamId
    if (previous) {
      retiredTranslationStreamIds.add(previous)
      while (retiredTranslationStreamIds.size > 32) {
        const oldest = retiredTranslationStreamIds.values().next().value
        if (!oldest) break
        retiredTranslationStreamIds.delete(oldest)
      }
    }
    activeTranslationStreamId = ''
    return previous
  }

  function handleTranslation(message: TranslationMessage) {
    if (!requestedTranslationEnabled()) return
    const route = translationMessageRoute(
      message,
      translationMessageChannelId(),
      activeTranslationStreamId,
      retiredTranslationStreamIds,
    )
    if (!route) return
    if (message.type === 'translation-status') {
      if (message.status === 'ready') {
        if (route.establishesStream) {
          activeTranslationStreamId = route.streamId
          translationHistories = clearTranslationDraft(translationHistories, route.channelId)
        }
        translationStatus = 'ready'
        lastTranslationErrorAt = 0
        addLog('VoiceTranslate is ready', 'success')
      } else if (message.status === 'error') {
        translationStatus = 'error'
        const errorText = translationErrorText(message.stage)
        const now = Date.now()
        if (now - lastTranslationErrorAt >= 10_000) {
          lastTranslationErrorAt = now
          addLog(errorText, 'error')
        }
      }
      if (translationStatusClearsDraft(message.status)) {
        translationHistories = clearTranslationDraft(translationHistories, route.channelId)
      }
      return
    }
    translationHistories = translationHistoryState(translationHistories, route.channelId, message, Date.now())
  }

  function translationErrorText(stage?: string): string {
    switch (stage) {
      case 'capacity': return 'VoiceTranslate capacity: Translation service is busy'
      case 'authentication': return 'VoiceTranslate authentication: Translation authentication failed'
      case 'connection': return 'VoiceTranslate connection: Translation service connection failed'
      case 'translation': return 'VoiceTranslate translation: Translation failed'
      case 'speech': return 'VoiceTranslate speech: Translated speech failed'
      default: return 'VoiceTranslate service: Translation service error'
    }
  }

  function updateSubtitleText() {
    activeCaptions = Object.values(activeSubtitles)
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
      activeSubtitles[id] = message
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

  function toggleTranslation() {
    const requestedTranslationEnabled = !translationEnabled
    const previousTranslationStreamId = retireActiveTranslationStream()
    addLog(`Translation: ${requestedTranslationEnabled ? 'ON' : 'OFF'}`)
    if (!requestedTranslationEnabled) clearCurrentTranslationDraft()
    if (rtcClient && streamStarted) {
      beginStartAttempt()
      const previousTranslationStatus = translationStatus
      translationStatus = requestedTranslationEnabled ? 'connecting' : 'off'
      const requestId = rtcClient.setTranslationEnabled(requestedTranslationEnabled)
      pendingRestart = {
        requestId,
        previousChannel: currentChannel,
        requestedChannel: currentChannel,
        previousAudioMode: audioMode,
        requestedAudioMode: audioMode,
        previousTranslationEnabled: translationEnabled,
        requestedTranslationEnabled,
        previousTranslationStatus,
        previousTranslationStreamId,
      }
    } else {
      translationEnabled = requestedTranslationEnabled
      translationStatus = requestedTranslationEnabled ? 'connecting' : 'off'
    }
  }

  // Cycle through audio modes: both -> main -> sub -> both
  function cycleAudioMode() {
    const modes: AudioMode[] = ['both', 'main', 'sub']
    const currentIndex = modes.indexOf(audioMode)
    const requestedAudioMode = modes[(currentIndex + 1) % modes.length]
    addLog(`Audio mode: ${getAudioModeLabel(requestedAudioMode)}`)
    // Restart encoding with new setting if currently streaming
    if (rtcClient && streamStarted) {
      beginStartAttempt()
      const previousTranslationStatus = translationStatus
      const previousTranslationStreamId = translationEnabled ? retireActiveTranslationStream() : ''
      if (translationEnabled) translationStatus = 'connecting'
      const requestId = rtcClient.setAudioMode(requestedAudioMode)
      pendingRestart = {
        requestId,
        previousChannel: currentChannel,
        requestedChannel: currentChannel,
        previousAudioMode: audioMode,
        requestedAudioMode,
        previousTranslationEnabled: translationEnabled,
        requestedTranslationEnabled: translationEnabled,
        previousTranslationStatus,
        previousTranslationStreamId,
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
    debugLogs = [logEntry, ...debugLogs.slice(0, 999)]
    console.log(message)
  }

  $: if (selectedChannel && videoElement) {
    if (!isStartingStream && selectedChannel !== currentChannel) {
      startStream(selectedChannel)
    }
  }

  // Flash the bar on every channel change, then let it fade away again.
  $: if (selectedChannel) revealControls()

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
      settleStartAttempt()
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

    beginStartAttempt()
    const startTime = Date.now()
    tracksReceived = 0

    const channelId = channelSelectionId(channel)

    addLog(`[${startTime}] Starting WebRTC stream for channel ${channelId} (${channel.name})`)

    // If we already have an active connection, use restartEncoding to switch channels
    if (rtcClient && connectionStatus === 'connected' && currentChannel) {
      addLog(`Switching channel from ${currentChannel.channel} to ${channelId} using restart-encoding`)
      clearSubtitles()
      const previousTranslationStatus = translationStatus
      const previousTranslationStreamId = translationEnabled ? retireActiveTranslationStream() : ''
      if (translationEnabled) translationStatus = 'connecting'
      const requestId = rtcClient.restartEncoding({ channelId, burnInSubtitles: true, audioMode, translationEnabled })
      pendingRestart = {
        requestId,
        previousChannel: currentChannel,
        requestedChannel: channel,
        previousAudioMode: audioMode,
        requestedAudioMode: audioMode,
        previousTranslationEnabled: translationEnabled,
        requestedTranslationEnabled: translationEnabled,
        previousTranslationStatus,
        previousTranslationStreamId,
      }
      return
    }

    if (translationEnabled) retireActiveTranslationStream()

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
      videoWidth = 0
      videoHeight = 0
    }

    connectionStatus = 'disconnected'

    try {
      // Create WebRTC client with the current subtitle setting.
      rtcClient = new RTCClient({
        channelId,
        burnInSubtitles: true,
        audioMode,
        translationEnabled,
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
          startWatchdogReleases = 0
          settleStartAttempt()
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
            settleStartAttempt()
          }
          scheduleRecovery(currentChannel, thisStreamSequence, state)
        },
        onError: (error, requestId) => {
          if (thisStreamSequence !== streamSequence || destroyed) return
          addLog(`WebRTC error: ${error.message}`, 'error')
          if (requestId && pendingRestart?.requestId === requestId) {
            retireActiveTranslationStream()
            selectedChannel = pendingRestart.previousChannel
            currentChannel = pendingRestart.previousChannel
            audioMode = pendingRestart.previousAudioMode
            translationEnabled = pendingRestart.previousTranslationEnabled
            translationStatus = pendingRestart.previousTranslationStatus
            activeTranslationStreamId = pendingRestart.previousTranslationStreamId
            retiredTranslationStreamIds.delete(activeTranslationStreamId)
            pendingRestart = null
          }
          settleStartAttempt()
        },
        onEncodingRestarted: (newChannelId, requestId) => {
          if (thisStreamSequence !== streamSequence || destroyed) return
          if (requestId && pendingRestart?.requestId === requestId) {
            currentChannel = pendingRestart.requestedChannel
            selectedChannel = pendingRestart.requestedChannel
            audioMode = pendingRestart.requestedAudioMode
            translationEnabled = pendingRestart.requestedTranslationEnabled
            translationStatus = translationStatusAfterRestart(translationStatus, translationEnabled)
            if (!translationEnabled) clearCurrentTranslationDraft()
            pendingRestart = null
          }
          addLog(`Encoding restarted for channel ${newChannelId}`, 'success')
          settleStartAttempt()
          schedulePipelineLogs(newChannelId)
        },
        onSubtitle: handleSubtitle,
        onTranslation: handleTranslation,
        onVideoFormat: applyVideoFormat,
        onLog: (message) => {
          debugLogs = [message, ...debugLogs.slice(0, 999)]
        }
      })

      // Connect
      await rtcClient.connect()
      currentSessionId = rtcClient.getPeerId() || channelId

      schedulePipelineLogs(channelId)

    } catch (error) {
      addLog(`Failed to start WebRTC stream: ${error}`, 'error')
      settleStartAttempt()
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
    window.removeEventListener('resize', updateFrameSize)
    window.removeEventListener('blur', cancelPlayerGesture)
    window.visualViewport?.removeEventListener('resize', updateFrameSize)
    orientationController()?.removeEventListener?.('change', scheduleViewportSettle)
    frameResizeObserver?.disconnect()
    cancelAnimationFrame(frameAnimationFrame)
    clearControlsTimer()
    clearViewportSettleTimers()
    orientationLockCoordinator?.destroy()
    orientationLockCoordinator = null
    clearSubtitles()
    if (translationPruneTimer) clearInterval(translationPruneTimer)
    clearStartWatchdog()
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

<!-- Full-bleed stage: the picture fills the viewport, one bar floats on top of it. -->
<div
  class="player-shell"
  role="presentation"
  class:controls-hidden={!controlsVisible}
  class:volume-control-open={showVolumeControl}
  bind:this={playerShell}
  on:pointerdown={handlePlayerPointerDown}
  on:pointerup={handlePlayerPointerUp}
  on:pointercancel={cancelPlayerGesture}
  on:lostpointercapture={cancelPlayerGesture}
  on:pointermove={revealControls}
>
  <div class="video-stage" bind:this={videoStage}>
    {#if selectedChannel}
      <div class="video-frame" style="--frame-width: {frameWidth ? `${frameWidth}px` : '100%'}; --frame-height: {frameHeight ? `${frameHeight}px` : '100%'}">
        <!-- svelte-ignore a11y-media-has-caption -->
        <video
          bind:this={videoElement}
          style:object-fit="fill"
          autoplay
          playsinline
          muted={!audioEnabled}
          on:volumechange={syncMediaVolume}
          on:resize={logIntrinsicSize}
          on:loadedmetadata={logIntrinsicSize}
          use:nativeFullscreenEvents
          on:play={() => {
            addLog(`Video play event, paused=${videoElement?.paused}, currentTime=${videoElement?.currentTime}`)
          }}
          on:playing={() => {
            logIntrinsicSize()
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
            logIntrinsicSize()
            addLog(`Video loadeddata, videoWidth=${videoWidth}, videoHeight=${videoHeight}`)
          }}
        >
        </video>

        {#if burnInSubtitles && activeCaptions.length}
          <SubtitleRenderer captions={activeCaptions} />
        {/if}

        {#if translationEnabled}
          <TranslationLogOverlay channelId={translationChannelId} log={activeTranslationLog} status={translationStatus} />
        {/if}

        {#if isStartingStream}
          <div class="stage-busy">
            <div>
              <div class="stage-spinner"></div>
              接続中
            </div>
          </div>
        {/if}
      </div>
    {:else}
      <p class="stage-placeholder">番組表からチャンネルを選んでください</p>
    {/if}
  </div>

  <div
    class="player-bar"
    role="group"
    aria-label="再生コントロール"
    on:pointerenter={handleBarPointerEnter}
    on:pointerleave={handleBarPointerLeave}
    on:pointerdown={revealControls}
    on:pointerup={revealControls}
    on:focusin={handleToolbarFocusIn}
    on:focusout={handleToolbarFocusOut}
  >
    <span class="bar-channel">
      <span class="bar-dot" class:live={connectionStatus === 'connected'}></span>
      <span>{currentChannel?.displayName || currentChannel?.name || selectedChannel?.displayName || selectedChannel?.name || '未選択'}</span>
    </span>

    <span class="bar-spacer"></span>
    <TunerStatus />

    <div class="bar-actions">
      <button class="bar-button" on:click={() => dispatch('openEPG')}>
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="3" y="4" width="18" height="17" />
          <path d="M3 9h18M8 3v3M16 3v3M8 14h4" />
        </svg>
        <span class="bar-text">番組表</span>
      </button>
      <button class="bar-button" on:click={() => dispatch('openChannels')}>
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="3" y="6" width="18" height="12" />
          <path d="M8 3l4 3 4-3" />
        </svg>
        <span class="bar-text">チャンネル</span>
      </button>
      <button class="bar-button" on:click={cycleAudioMode} disabled={isStartingStream}>
        <span class="bar-label">音声</span>
        <strong>{getAudioModeLabel(audioMode)}</strong>
      </button>
      <button class="bar-button" class:active={burnInSubtitles} on:click={toggleSubtitles} disabled={isStartingStream} aria-pressed={burnInSubtitles}>
        <span class="bar-label">字幕</span>
        <strong>{burnInSubtitles ? '入' : '切'}</strong>
      </button>
      <button
        class="bar-button"
        class:active={translationEnabled}
        class:translation-error={translationStatus === 'error'}
        on:click={toggleTranslation}
        disabled={isStartingStream || !streamStarted}
        aria-pressed={translationEnabled}
        aria-label="遅延する日本語翻訳字幕と翻訳音声を切り替え"
        title="認識後に数秒遅れて再生 · 日本語字幕・VOICEVOX:ずんだもん音声"
      >
        <span class="bar-label">遅延翻訳</span>
        <strong>{translationEnabled ? '入' : '切'}</strong>
      </button>
      <div class="volume-control">
        <button
          class="bar-button"
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
          <span class="bar-text">{audioEnabled ? `${Math.round(volume * 100)}%` : '消音'}</span>
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
      <button class="bar-button" on:click={toggleFullscreen} aria-label={isFullscreen ? '全画面を終了' : '全画面で見る'}>
        <svg viewBox="0 0 24 24" aria-hidden="true">
          {#if isFullscreen}
            <path d="M9 3v6H3M15 3v6h6M9 21v-6H3M15 21v-6h6" />
          {:else}
            <path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5" />
          {/if}
        </svg>
      </button>
      <button class="bar-button" on:click={() => dispatch('openSettings')} aria-label="設定を開く">
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="3" />
          <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6L17 7M7 17l-1.4 1.4" />
        </svg>
      </button>
    </div>
  </div>
</div>
