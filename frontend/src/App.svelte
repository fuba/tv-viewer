<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import VideoPlayer from './components/VideoPlayer.svelte'
  import ChannelList from './components/ChannelList.svelte'
  import ProgramGuide from './components/ProgramGuide.svelte'
  import OverlayPanel from './components/OverlayPanel.svelte'
  import MetaBar from './components/MetaBar.svelte'
  import SettingsPanel from './components/SettingsPanel.svelte'
  import EPGGrid from './components/EPGGrid.svelte'
  import type { ConnectionStatus } from './lib/webrtc/types'
  import { channelMatchesSelection, resolveActiveChannel } from './lib/channelSelection'

  let selectedChannel: any = null
  let showChannelPanel = true  // Open channel list by default
  let showSettingsPanel = false
  let showEPGPanel = false
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
          showChannelPanel = false
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
  }
</script>

<div class="app-shell">
  <header class="topbar">
    <div class="brand">
      <span class="brand-mark"></span>
      <span>TV</span>
    </div>
    <nav class="topbar-actions" aria-label="メインメニュー">
      <button
        class="topbar-button"
        on:click={() => showEPGPanel = true}
      >
        <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
        </svg>
        <span>番組表</span>
      </button>
      <button
        class="topbar-button"
        on:click={() => showChannelPanel = true}
      >
        <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>チャンネル</span>
      </button>
    </nav>
  </header>

  <main class="viewer-main">
    <div class="viewer-frame">
      <VideoPlayer
        bind:this={videoPlayer}
        bind:selectedChannel
        bind:debugLogs
        bind:pipelineLogs
        bind:connectionStatus
        bind:streamStarted
      />
    </div>
  </main>

  <MetaBar
    channel={selectedChannel}
    {connectionStatus}
    on:channelClick={() => showChannelPanel = true}
    on:settingsClick={() => showSettingsPanel = true}
  />

  <!-- Channel selection overlay -->
  <OverlayPanel bind:isOpen={showChannelPanel} title="チャンネル選択">
    <div class="panel-stack">
      <ChannelList bind:selectedChannel {activeChannelId} {activeViewerCount} on:select={handleChannelSelect} />
      <div class="panel-divider">
        <ProgramGuide channel={selectedChannel} on:select={handleEPGSelect} />
      </div>
    </div>
  </OverlayPanel>

  <!-- Settings overlay -->
  <OverlayPanel bind:isOpen={showSettingsPanel} title="設定">
    <SettingsPanel
      {selectedChannel}
      bind:debugLogs
      bind:pipelineLogs
      {streamStarted}
    />
  </OverlayPanel>

  <!-- EPG (Program Guide Grid) overlay -->
  <OverlayPanel bind:isOpen={showEPGPanel} title="番組表" fullWidth={true}>
    <EPGGrid on:select={handleEPGSelect} />
  </OverlayPanel>
</div>
