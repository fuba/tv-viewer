<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Hls from 'hls.js'
  import SubtitleRenderer from './SubtitleRenderer.svelte'
  
  export let selectedChannel: any
  
  let videoElement: HTMLVideoElement
  let hls: Hls | null = null
  let streamStarted = false
  
  $: if (selectedChannel && videoElement) {
    startStream(selectedChannel)
  }
  
  async function startStream(channel: any) {
    streamStarted = false
    
    // First start the encoding
    try {
      const response = await fetch(`/api/channels/${channel.id}/stream`, {
        method: 'GET'
      })
      
      if (response.ok) {
        const data = await response.json()
        console.log('Stream started:', data)
        
        // Wait a bit for the stream to initialize
        setTimeout(() => {
          loadVideo(channel)
          streamStarted = true
        }, 2000)
      }
    } catch (error) {
      console.error('Failed to start stream:', error)
    }
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

<div class="bg-black rounded-lg overflow-hidden aspect-video relative">
  {#if selectedChannel}
    <video
      bind:this={videoElement}
      class="w-full h-full"
      controls
      autoplay
    >
    </video>
    {#if streamStarted}
      <SubtitleRenderer
        subtitleUrl={`/api/stream/${selectedChannel.id}/subtitles.ass`}
        {videoElement}
      />
    {/if}
  {:else}
    <div class="flex items-center justify-center h-full text-gray-500">
      <p class="text-xl">チャンネルを選択してください</p>
    </div>
  {/if}
</div>