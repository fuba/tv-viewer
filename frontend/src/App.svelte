<script lang="ts">
  import VideoPlayer from './components/VideoPlayer.svelte'
  import ChannelList from './components/ChannelList.svelte'
  import ProgramGuide from './components/ProgramGuide.svelte'
  
  let selectedChannel: any = null
  let showDebug: boolean = false
</script>

<main class="min-h-screen bg-gray-900 text-white">
  <div class="container mx-auto p-4">
    <div class="flex justify-between items-center mb-6">
      <h1 class="text-3xl font-bold">TV Viewer</h1>
      <button 
        class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
        on:click={() => showDebug = !showDebug}
      >
        {showDebug ? 'デバッグ非表示' : 'デバッグ表示'}
      </button>
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="lg:col-span-2">
        <VideoPlayer {selectedChannel} {showDebug} />
      </div>
      
      <div class="space-y-6">
        <ChannelList bind:selectedChannel />
        <ProgramGuide channel={selectedChannel} />
      </div>
    </div>
  </div>
</main>