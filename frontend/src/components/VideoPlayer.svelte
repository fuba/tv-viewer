<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Hls from 'hls.js'
  import SubtitleRenderer from './SubtitleRenderer.svelte'
  
  export let selectedChannel: any
  export let showDebug: boolean = false
  
  let videoElement: HTMLVideoElement
  let hls: Hls | null = null
  let streamStarted = false
  let debugLogs: string[] = []
  let currentChannel: any = null
  let isStartingStream = false
  let ffmpegLogs: string[] = []
  
  function addLog(message: string, type: 'info' | 'error' | 'success' = 'info') {
    const timestamp = new Date().toLocaleTimeString()
    const logEntry = `[${timestamp}] ${message}`
    debugLogs = [logEntry, ...debugLogs.slice(0, 99999)] // Keep last 100000 logs
    console.log(message)
  }
  
  $: if (selectedChannel && videoElement) {
    // Prevent duplicate requests for the same channel
    if (!isStartingStream && selectedChannel !== currentChannel) {
      startStream(selectedChannel)
    }
  }
  
  async function startStream(channel: any) {
    if (isStartingStream) {
      addLog(`Stream already starting, ignoring duplicate request`)
      return
    }
    
    isStartingStream = true
    currentChannel = channel
    const startTime = Date.now()
    streamStarted = false
    addLog(`[${startTime}] Starting stream for channel ${channel.channel} (${channel.name})`)
    
    // Stop existing stream first
    if (hls) {
      hls.destroy()
      hls = null
      addLog(`[${Date.now() - startTime}ms] Stopped existing HLS stream`)
    }
    
    // Clear video source to prevent old content from showing
    if (videoElement) {
      videoElement.src = ''
      videoElement.load() // Force reload to clear any cached content
      addLog(`[${Date.now() - startTime}ms] Cleared video element`)
    }
    
    // Use channel number since id is empty
    const channelId = channel.channel || channel.id
    const encodedChannelId = encodeURIComponent(channelId)
    
    // First start the encoding
    try {
      addLog(`[${Date.now() - startTime}ms] Requesting encoding start for channel ${channelId}`)
      const response = await fetch(`/api/channels/${encodedChannelId}/stream`, {
        method: 'GET'
      })
      
      if (response.ok) {
        const data = await response.json()
        addLog(`[${Date.now() - startTime}ms] Stream started: ${data.sessionId}`, 'success')
        
        // Start fetching FFmpeg logs
        fetchFFmpegLogs(channel)
        
        // Poll for playlist availability with increased retries
        let retries = 0
        const maxRetries = 30 // Increased retry count
        const checkPlaylist = async () => {
          try {
            const checkTime = Date.now()
            const playlistResponse = await fetch(`/api/stream/${encodedChannelId}/playlist.m3u8?t=${checkTime}`)
            addLog(`[${checkTime - startTime}ms] Checking playlist: status ${playlistResponse.status}`)
            
            if (playlistResponse.ok) {
              // Verify playlist content is valid
              const playlistContent = await playlistResponse.text()
              if (playlistContent.includes('#EXTM3U') && playlistContent.includes('.ts')) {
                addLog(`[${Date.now() - startTime}ms] Valid playlist ready, loading video`, 'success')
                loadVideo(channel)
                streamStarted = true
                isStartingStream = false
                return
              } else {
                addLog(`[${Date.now() - startTime}ms] Playlist exists but content not ready yet - Length: ${playlistContent.length}`)
              }
            }
            
            if (retries < maxRetries) {
              retries++
              addLog(`Playlist not ready, retrying... (${retries}/${maxRetries}) - HTTP ${playlistResponse.status}`)
              // Very aggressive retry timing
              const baseDelay = 200
              const delay = Math.min(baseDelay + (retries * 25), 1500)
              setTimeout(checkPlaylist, delay)
            } else {
              addLog(`Playlist not available after ${maxRetries} retries`, 'error')
              isStartingStream = false
            }
          } catch (error) {
            if (retries < maxRetries) {
              retries++
              addLog(`Error checking playlist (${retries}/${maxRetries}): ${error}`)
              setTimeout(checkPlaylist, 1000)
            } else {
              addLog(`Failed to check playlist after ${maxRetries} retries: ${error}`, 'error')
              isStartingStream = false
            }
          }
        }
        
        // Start checking immediately
        checkPlaylist()
      } else {
        addLog(`Failed to start stream: HTTP ${response.status}`, 'error')
        isStartingStream = false
      }
    } catch (error) {
      addLog(`Failed to start stream: ${error}`, 'error')
      isStartingStream = false
    }
  }
  
  function loadVideo(channel: any) {
    if (hls) {
      hls.destroy()
    }
    
    // Clear video element completely
    if (videoElement) {
      videoElement.src = ''
      videoElement.load()
    }
    
    if (Hls.isSupported()) {
      hls = new Hls({
        debug: true,
        enableWorker: false,
        lowLatencyMode: true,
        backBufferLength: 6,   // 3 segments * 2 seconds
        maxBufferLength: 12,   // 6 segments * 2 seconds  
        manifestLoadingTimeOut: 10000,
        manifestLoadingRetryDelay: 500, // Faster retries for 2-second segments
        manifestLoadingMaxRetry: 20,
        levelLoadingTimeOut: 10000,
        fragLoadingTimeOut: 10000,
        // Prevent caching issues
        xhrSetup: function(xhr: XMLHttpRequest, url: string) {
          // Add cache-busting query parameter to all requests
          const separator = url.includes('?') ? '&' : '?'
          const cacheParam = `_t=${Date.now()}&_r=${Math.random()}`
          xhr.open('GET', url + separator + cacheParam, true)
          // Force no-cache headers
          xhr.setRequestHeader('Cache-Control', 'no-cache, no-store, must-revalidate')
          xhr.setRequestHeader('Pragma', 'no-cache')
          xhr.setRequestHeader('Expires', '0')
        }
      })
      
      const channelId = channel.channel || channel.id
      const encodedChannelId = encodeURIComponent(channelId)
      
      hls.on(Hls.Events.ERROR, function (event, data) {
        addLog(`HLS error: ${data.type} - ${data.details}`, 'error')
        if (data.fatal) {
          switch(data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
              addLog('Network error, retrying in 1 second...')
              setTimeout(() => {
                hls.startLoad()
              }, 1000)
              break
            case Hls.ErrorTypes.MEDIA_ERROR:
              addLog('Media error, attempting recovery...')
              hls.recoverMediaError()
              break
            default:
              addLog('Fatal HLS error, destroying instance', 'error')
              hls.destroy()
              break
          }
        }
      })
      
      hls.on(Hls.Events.MANIFEST_LOADED, function(event, data) {
        addLog('HLS manifest loaded successfully', 'success')
      })
      
      hls.on(Hls.Events.LEVEL_LOADED, function(event, data) {
        addLog(`HLS level loaded: ${data.details.fragments.length} fragments`, 'success')
      })
      
      hls.on(Hls.Events.FRAG_LOADED, function(event, data) {
        addLog(`Fragment loaded: ${data.frag.relurl}`)
      })
      
      addLog(`Loading HLS source: /api/stream/${encodedChannelId}/playlist.m3u8`)
      
      // Clear any existing data before loading new source
      hls.attachMedia(videoElement)
      hls.loadSource(`/api/stream/${encodedChannelId}/playlist.m3u8`)
      
      // Start loading immediately
      hls.startLoad()
    } else if (videoElement.canPlayType('application/vnd.apple.mpegurl')) {
      const channelId = channel.channel || channel.id
      const encodedChannelId = encodeURIComponent(channelId)
      addLog('Using native HLS support')
      videoElement.src = `/api/stream/${encodedChannelId}/playlist.m3u8`
    } else {
      addLog('HLS not supported in this browser', 'error')
    }
  }
  
  async function fetchFFmpegLogs(channel: any) {
    try {
      const channelId = channel.channel || channel.id
      const encodedChannelId = encodeURIComponent(channelId)
      const response = await fetch(`/api/logs/${encodedChannelId}`)
      
      if (response.ok) {
        const data = await response.json()
        ffmpegLogs = data.logs || []
        addLog(`FFmpeg logs fetched: ${ffmpegLogs.length} entries`)
      }
    } catch (error) {
      addLog(`Failed to fetch FFmpeg logs: ${error}`, 'error')
    }
    
    // Continue fetching logs every 2 seconds while streaming
    if (streamStarted) {
      setTimeout(() => fetchFFmpegLogs(channel), 2000)
    }
  }

  onDestroy(() => {
    if (hls) {
      hls.destroy()
    }
  })
</script>

<div class="space-y-4">
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
          subtitleUrl={`/api/stream/${selectedChannel.channel || selectedChannel.id}/subtitles.ass`}
          {videoElement}
        />
      {/if}
    {:else}
      <div class="flex items-center justify-center h-full text-gray-500">
        <p class="text-xl">チャンネルを選択してください</p>
      </div>
    {/if}
  </div>

  {#if showDebug}
    <div class="bg-gray-900 rounded-lg p-4 space-y-4">
      <!-- Application Debug Logs -->
      <div>
        <div class="flex justify-between items-center mb-2">
          <h3 class="text-lg font-semibold text-white">アプリケーションログ</h3>
          <button 
            class="px-3 py-1 bg-red-600 text-white rounded text-sm hover:bg-red-700"
            on:click={() => debugLogs = []}
          >
            クリア
          </button>
        </div>
        <div class="bg-black rounded p-3 h-64 overflow-y-auto">
          {#if debugLogs.length === 0}
            <p class="text-gray-400 text-sm">ログはありません</p>
          {:else}
            {#each debugLogs as log}
              <div class="text-xs font-mono mb-1 text-green-400">
                {log}
              </div>
            {/each}
          {/if}
        </div>
      </div>
      
      <!-- FFmpeg Logs -->
      <div>
        <div class="flex justify-between items-center mb-2">
          <h3 class="text-lg font-semibold text-white">FFmpegログ</h3>
          <button 
            class="px-3 py-1 bg-red-600 text-white rounded text-sm hover:bg-red-700"
            on:click={() => ffmpegLogs = []}
          >
            クリア
          </button>
        </div>
        <div class="bg-black rounded p-3 h-64 overflow-y-auto">
          {#if ffmpegLogs.length === 0}
            <p class="text-gray-400 text-sm">FFmpegログはありません</p>
          {:else}
            {#each ffmpegLogs as log}
              <div class="text-xs font-mono mb-1 text-yellow-400 whitespace-pre-wrap">
                {log}
              </div>
            {/each}
          {/if}
        </div>
      </div>
      
      <div class="text-xs text-gray-400">
        チャンネル: {selectedChannel?.name || 'なし'} | 
        ストリーミング状態: {streamStarted ? '再生中' : '待機中'}
      </div>
    </div>
  {/if}
</div>