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

<main class="min-h-screen bg-gray-900 text-white">
  <!-- Compact header -->
  <header class="flex justify-between items-center px-4 py-2 bg-gray-800 border-b border-gray-700 h-14">
    <h1 class="text-xl font-bold">TV Viewer</h1>
    <button 
      class="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
      on:click={() => showDebug = !showDebug}
    >
      {showDebug ? 'Debug OFF' : 'Debug ON'}
    </button>
  </header>
  
  <!-- Main content area with CSS Grid -->
  <main class="grid gap-4 p-4" style="height: calc(100vh - 3.5rem);">
    <div class="
      grid gap-4 h-full
      grid-cols-1 grid-rows-[auto_1fr_1fr] sm:grid-rows-[auto_1fr_1fr] 
      md:grid-cols-[2fr_1fr] md:grid-rows-[auto_1fr]
      lg:grid-cols-[65fr_35fr] lg:grid-rows-[auto_1fr]
      xl:grid-cols-[70fr_30fr]
      max-w-[1600px] mx-auto w-full
    ">
      
      <!-- Video player container with size constraints -->
      <div class="
        row-start-1 col-start-1 
        md:row-start-1 md:col-start-1 md:row-span-2
        min-h-0 flex flex-col
      ">
        <div class="
          w-full h-full max-h-[70vh] 
          md:max-h-[calc(100vh-7rem)] 
          flex flex-col justify-center
        ">
          <VideoPlayer {selectedChannel} bind:debugLogs bind:ffmpegLogs />
        </div>
      </div>
      
      <!-- Information panel container -->
      <div class="
        row-start-2 col-start-1
        md:row-start-1 md:col-start-2 md:row-span-2
        min-h-0 flex flex-col gap-4 h-full
      ">
        {#if showDebug}
          <!-- Debug logs view -->
          <div class="h-full min-h-0">
            <DebugLogs bind:debugLogs bind:ffmpegLogs {selectedChannel} streamStarted={true} />
          </div>
        {:else}
          <!-- Channel list -->
          <div class="
            flex-1 min-h-0
            h-64 md:h-auto md:flex-[3]
          ">
            <ChannelList bind:selectedChannel />
          </div>
          
          <!-- Program guide -->
          <div class="
            flex-1 min-h-0
            h-64 md:h-auto md:flex-[2]
          ">
            <ProgramGuide channel={selectedChannel} />
          </div>
        {/if}
      </div>
    </div>
  </main>
</main>