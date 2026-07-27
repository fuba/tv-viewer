<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import VideoPlayer from './components/VideoPlayer.svelte'
  import ChannelList from './components/ChannelList.svelte'
  import ProgramGuide from './components/ProgramGuide.svelte'
  import OverlayPanel from './components/OverlayPanel.svelte'
  import SettingsPanel from './components/SettingsPanel.svelte'
  import EPGGrid from './components/EPGGrid.svelte'
  import type { ConnectionStatus } from './lib/webrtc/types'
  import { channelMatchesSelection, resolveActiveChannel } from './lib/channelSelection'

  let selectedChannel: any = null
  let showChannelPanel = false
  let showSettingsPanel = false
  let showEPGPanel = true  // The guide is the entry point, not the channel list
  let debugLogs: string[] = []
  let pipelineLogs: string[] = []
  let connectionStatus: ConnectionStatus = 'disconnected'
  let streamStarted = false
  let videoPlayer: VideoPlayer
  let activeChannelId = ''
  let activeViewerCount = 0
  let activeStatusTimer: ReturnType<typeof setInterval> | null = null
  let activeRefreshGeneration = 0

  async function refreshActiveChannel(autoSelect = false) {
	const generation = ++activeRefreshGeneration
    try {
      const statusResponse = await fetch('/api/webrtc/status')
      if (!statusResponse.ok) return
      const status = await statusResponse.json()
	  if (generation !== activeRefreshGeneration) return
      activeChannelId = status.activeChannelId || ''
      activeViewerCount = status.activeViewerCount || 0
      if (autoSelect && activeChannelId && !selectedChannel) {
		const requestedChannelId = activeChannelId
        const channelsResponse = await fetch('/api/channels')
        if (!channelsResponse.ok) return
		const channels = await channelsResponse.json()
		if (generation !== activeRefreshGeneration || selectedChannel || activeChannelId !== requestedChannelId) return
		const channel = resolveActiveChannel(channels, requestedChannelId)
        if (channel) {
          selectedChannel = channel
          // A stream is already running: go straight to the picture.
          showChannelPanel = false
          showEPGPanel = false
        }
      }
    } catch (error) {
      console.error('Failed to refresh active channel:', error)
    }
  }

  onMount(() => {
    void refreshActiveChannel(true)
    activeStatusTimer = setInterval(() => void refreshActiveChannel(true), 2000)
  })

  onDestroy(() => {
    if (activeStatusTimer) clearInterval(activeStatusTimer)
  })

  function handleChannelSelect() {
    videoPlayer?.enableAudioFromUserGesture()
    showChannelPanel = false
  }

  function handleEPGSelect(event: CustomEvent) {
    if (activeViewerCount > 1 && !channelMatchesSelection(event.detail, activeChannelId)) {
      debugLogs = [`[${new Date().toLocaleTimeString()}] Channel is locked by other viewers`, ...debugLogs]
      return
    }
    videoPlayer?.enableAudioFromUserGesture()
    selectedChannel = event.detail
    showEPGPanel = false
    showChannelPanel = false
  }
</script>

<div class="app-shell">
  <VideoPlayer
    bind:this={videoPlayer}
    bind:selectedChannel
    bind:debugLogs
    bind:pipelineLogs
    bind:connectionStatus
    bind:streamStarted
    on:openEPG={() => showEPGPanel = !showEPGPanel}
    on:openChannels={() => showChannelPanel = !showChannelPanel}
    on:openSettings={() => showSettingsPanel = !showSettingsPanel}
  />

  <!-- Program guide (primary entry point) -->
  <OverlayPanel bind:isOpen={showEPGPanel} title="番組表">
    <EPGGrid on:select={handleEPGSelect} />
  </OverlayPanel>

  <!-- Channel selection overlay -->
  <OverlayPanel bind:isOpen={showChannelPanel} title="チャンネル">
    <div class="panel-body panel-stack">
      <ChannelList bind:selectedChannel {activeChannelId} {activeViewerCount} on:select={handleChannelSelect} />
      <div class="panel-divider">
        <ProgramGuide channel={selectedChannel} on:select={handleEPGSelect} />
      </div>
    </div>
  </OverlayPanel>

  <!-- Settings overlay -->
  <OverlayPanel bind:isOpen={showSettingsPanel} title="設定">
    <div class="panel-body">
      <SettingsPanel
        {selectedChannel}
        bind:debugLogs
        bind:pipelineLogs
        {streamStarted}
      />
    </div>
  </OverlayPanel>
</div>
