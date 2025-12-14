<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Hls from 'hls.js'
  
  export let selectedChannel: any
  
  let videoElement: HTMLVideoElement
  let hls: Hls | null = null
  
  $: if (selectedChannel && videoElement) {
    loadVideo(selectedChannel)
  }
  
  function loadVideo(channel: any) {
    if (hls) {
      hls.destroy()
    }
    
    if (Hls.isSupported()) {
      hls = new Hls()
      hls.loadSource(`/api/stream/${channel.id}/playlist.m3u8`)
      hls.attachMedia(videoElement)
    } else if (videoElement.canPlayType('application/vnd.apple.mpegurl')) {
      videoElement.src = `/api/stream/${channel.id}/playlist.m3u8`
    }
  }
  
  onDestroy(() => {
    if (hls) {
      hls.destroy()
    }
  })
</script>

<div class="bg-black rounded-lg overflow-hidden aspect-video">
  {#if selectedChannel}
    <video
      bind:this={videoElement}
      class="w-full h-full"
      controls
      autoplay
    >
    </video>
  {:else}
    <div class="flex items-center justify-center h-full text-gray-500">
      <p class="text-xl">チャンネルを選択してください</p>
    </div>
  {/if}
</div>