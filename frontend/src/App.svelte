<script lang="ts">
  import VideoPlayer from './components/VideoPlayer.svelte'
  import ChannelList from './components/ChannelList.svelte'
  import ProgramGuide from './components/ProgramGuide.svelte'
  import DebugLogs from './components/DebugLogs.svelte'
  
  let selectedChannel: any = null
  let showDebug: boolean = false
  let debugLogs: string[] = []
  let ffmpegLogs: string[] = []
</script>

<main class="h-screen bg-gray-900 text-white overflow-hidden">
  <div class="h-full flex flex-col">
    <!-- Compact header -->
    <div class="flex justify-between items-center px-4 py-2 bg-gray-800 border-b border-gray-700">
      <h1 class="text-xl font-bold">TV Viewer</h1>
      <button 
        class="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
        on:click={() => showDebug = !showDebug}
      >
        {showDebug ? 'Debug OFF' : 'Debug ON'}
      </button>
    </div>
    
    <!-- Main content area -->
    <div class="flex-1 flex flex-col lg:flex-row gap-4 p-4 overflow-hidden">
      <!-- Video player - full width on mobile, flex-1 on desktop -->
      <div class="flex-1 flex flex-col min-h-0">
        <VideoPlayer {selectedChannel} bind:debugLogs bind:ffmpegLogs />
      </div>
      
      <!-- Sidebar/Bottom section - shows channels OR debug logs -->
      {#if showDebug}
        <!-- Debug logs view - takes full sidebar space -->
        <div class="w-full lg:w-96 h-96 lg:h-full">
          <DebugLogs bind:debugLogs bind:ffmpegLogs {selectedChannel} streamStarted={true} />
        </div>
      {:else}
        <!-- Normal channel and program guide view -->
        <div class="w-full lg:w-96 flex flex-row lg:flex-col gap-4 h-96 lg:h-full">
          <!-- Channel list - takes more space on mobile -->
          <div class="flex-[2] lg:flex-1 min-h-0">
            <ChannelList bind:selectedChannel />
          </div>
          
          <!-- Program guide - narrower on mobile -->
          <div class="flex-1 lg:h-48">
            <ProgramGuide channel={selectedChannel} />
          </div>
        </div>
      {/if}
    </div>
  </div>
</main>