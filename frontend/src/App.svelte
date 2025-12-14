<script lang="ts">
  import VideoPlayer from './components/VideoPlayer.svelte'
  import ChannelList from './components/ChannelList.svelte'
  import ProgramGuide from './components/ProgramGuide.svelte'
  
  let selectedChannel: any = null
</script>

<main class="min-h-screen bg-gray-900 text-white">
  <div class="container mx-auto p-4">
    <h1 class="text-3xl font-bold mb-6">TV Viewer</h1>
    
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="lg:col-span-2">
        <VideoPlayer {selectedChannel} />
      </div>
      
      <div class="space-y-6">
        <ChannelList bind:selectedChannel />
        <ProgramGuide channel={selectedChannel} />
      </div>
    </div>
  </div>
</main>