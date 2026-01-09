<script lang="ts">
  import VideoPlayer from './components/VideoPlayer.svelte'
  import ChannelList from './components/ChannelList.svelte'
  import ProgramGuide from './components/ProgramGuide.svelte'
  import OverlayPanel from './components/OverlayPanel.svelte'
  import MetaBar from './components/MetaBar.svelte'
  import SettingsPanel from './components/SettingsPanel.svelte'
  import EPGGrid from './components/EPGGrid.svelte'
  import type { ConnectionStatus } from './lib/webrtc/types'

  let selectedChannel: any = null
  let showChannelPanel = false
  let showSettingsPanel = false
  let showEPGPanel = false
  let debugLogs: string[] = []
  let ffmpegLogs: string[] = []
  let connectionStatus: ConnectionStatus = 'disconnected'
  let streamStarted = false

  function handleChannelSelect() {
    showChannelPanel = false
  }

  function handleEPGSelect(event: CustomEvent) {
    selectedChannel = event.detail
    showEPGPanel = false
  }
</script>

<div class="min-h-screen bg-gray-900 text-white flex flex-col">
  <!-- Compact header -->
  <header class="h-12 flex items-center justify-between px-4 bg-gray-800/80 backdrop-blur border-b border-gray-700 z-20">
    <div class="font-bold text-lg">TV Viewer</div>
    <div class="flex items-center gap-2">
      <button
        class="flex items-center gap-1 px-3 py-1.5 text-sm rounded hover:bg-gray-700 transition-colors"
        on:click={() => showEPGPanel = true}
      >
        <!-- Grid/Table icon -->
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
        </svg>
        <span class="hidden sm:inline">番組表</span>
      </button>
      <button
        class="flex items-center gap-1 px-3 py-1.5 text-sm rounded hover:bg-gray-700 transition-colors"
        on:click={() => showChannelPanel = true}
      >
        <!-- TV icon -->
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span class="hidden sm:inline">チャンネル</span>
      </button>
    </div>
  </header>

  <!-- Main content (video player) - theater mode -->
  <main class="flex-1 flex flex-col pb-12 bg-black">
    <div class="flex-1 flex items-center justify-center">
      <VideoPlayer
        bind:selectedChannel
        bind:debugLogs
        bind:ffmpegLogs
        bind:connectionStatus
        bind:streamStarted
      />
    </div>
  </main>

  <!-- Bottom meta bar -->
  <MetaBar
    channel={selectedChannel}
    {connectionStatus}
    on:channelClick={() => showChannelPanel = true}
    on:settingsClick={() => showSettingsPanel = true}
  />

  <!-- Channel selection overlay -->
  <OverlayPanel bind:isOpen={showChannelPanel} title="チャンネル選択">
    <div class="space-y-4">
      <ChannelList bind:selectedChannel on:select={handleChannelSelect} />
      <div class="border-t border-gray-700 pt-4">
        <ProgramGuide channel={selectedChannel} />
      </div>
    </div>
  </OverlayPanel>

  <!-- Settings overlay -->
  <OverlayPanel bind:isOpen={showSettingsPanel} title="設定">
    <SettingsPanel
      {selectedChannel}
      bind:debugLogs
      bind:ffmpegLogs
      {streamStarted}
    />
  </OverlayPanel>

  <!-- EPG (Program Guide Grid) overlay -->
  <OverlayPanel bind:isOpen={showEPGPanel} title="番組表" fullWidth={true}>
    <EPGGrid on:select={handleEPGSelect} />
  </OverlayPanel>
</div>
