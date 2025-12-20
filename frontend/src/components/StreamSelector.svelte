<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  
  export let selectedChannel: any
  export let show: boolean = false
  
  interface StreamInfo {
    channel_id: string
    session_id: string
    stream_url: string
    video_stream_index: number
    audio_stream_index: number
    stream_info?: {
      streams: Stream[]
      format: any
    }
  }
  
  interface Stream {
    index: number
    codec_name: string
    codec_long_name: string
    codec_type: string
    width?: number
    height?: number
    channels?: number
    channel_layout?: string
    sample_rate?: string
    bit_rate?: string
    tags?: { [key: string]: string }
  }
  
  let streamInfo: StreamInfo | null = null
  let isLoading = false
  let error: string | null = null
  let selectedVideoStream: number = -1
  let selectedAudioStream: number = -1
  let refreshInterval: number | null = null
  
  // Filter streams by type
  function getStreamsByType(streams: Stream[], type: string): Stream[] {
    return streams.filter(s => s.codec_type === type)
  }
  
  // Get stream description
  function getStreamDescription(stream: Stream): string {
    switch (stream.codec_type) {
      case 'video':
        let desc = `#${stream.index}: ${stream.codec_long_name || stream.codec_name}`
        if (stream.width && stream.height) {
          desc += ` (${stream.width}x${stream.height})`
        }
        if (stream.bit_rate) {
          desc += `, ${formatBitrate(stream.bit_rate)}`
        }
        if (stream.tags?.language) {
          desc += `, ${stream.tags.language}`
        }
        return desc
      
      case 'audio':
        let audioDesc = `#${stream.index}: ${stream.codec_long_name || stream.codec_name}`
        if (stream.channel_layout) {
          audioDesc += ` (${stream.channel_layout})`
        } else if (stream.channels) {
          audioDesc += ` (${stream.channels}ch)`
        }
        if (stream.sample_rate) {
          audioDesc += `, ${stream.sample_rate} Hz`
        }
        if (stream.bit_rate) {
          audioDesc += `, ${formatBitrate(stream.bit_rate)}`
        }
        if (stream.tags?.language) {
          audioDesc += `, ${stream.tags.language}`
        }
        return audioDesc
      
      case 'subtitle':
        let subDesc = `#${stream.index}: ${stream.codec_long_name || stream.codec_name}`
        if (stream.tags?.language) {
          subDesc += ` (${stream.tags.language})`
        }
        return subDesc
      
      default:
        return `#${stream.index}: ${stream.codec_type} - ${stream.codec_name}`
    }
  }
  
  // Format bitrate for display
  function formatBitrate(bitrate: string): string {
    const bps = parseInt(bitrate)
    if (isNaN(bps)) return bitrate
    
    if (bps >= 1000000) {
      return `${(bps / 1000000).toFixed(1)} Mbps`
    } else if (bps >= 1000) {
      return `${(bps / 1000).toFixed(0)} Kbps`
    } else {
      return `${bps} bps`
    }
  }
  
  async function fetchStreamInfo() {
    if (!selectedChannel || !show) return
    
    isLoading = true
    error = null
    
    try {
      const channelId = selectedChannel.channel || selectedChannel.id
      const encodedChannelId = encodeURIComponent(channelId)
      const response = await fetch(`/api/channels/${encodedChannelId}/stream-info`)
      
      if (response.ok) {
        streamInfo = await response.json()
        // Set current selection
        if (streamInfo) {
          selectedVideoStream = streamInfo.video_stream_index
          selectedAudioStream = streamInfo.audio_stream_index
        }
      } else if (response.status === 404) {
        error = 'No active stream for this channel'
        streamInfo = null
      } else {
        error = `Failed to fetch stream info: ${response.statusText}`
      }
    } catch (e) {
      error = `Error: ${e.message}`
    } finally {
      isLoading = false
    }
  }
  
  async function applyStreamSelection() {
    if (!selectedChannel || selectedVideoStream === undefined || selectedAudioStream === undefined) return
    
    isLoading = true
    error = null
    
    try {
      const channelId = selectedChannel.channel || selectedChannel.id
      const encodedChannelId = encodeURIComponent(channelId)
      
      const response = await fetch(`/api/channels/${encodedChannelId}/select-streams`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          video_stream_index: selectedVideoStream,
          audio_stream_index: selectedAudioStream
        })
      })
      
      if (response.ok) {
        // Stream selection updated successfully
        error = null
        // Fetch updated info
        await fetchStreamInfo()
      } else {
        const data = await response.json()
        error = data.error || 'Failed to update stream selection'
      }
    } catch (e) {
      error = `Error: ${e.message}`
    } finally {
      isLoading = false
    }
  }
  
  // Auto-refresh stream info when visible
  $: if (show && selectedChannel) {
    fetchStreamInfo()
    // Refresh every 5 seconds
    if (refreshInterval) clearInterval(refreshInterval)
    refreshInterval = setInterval(fetchStreamInfo, 5000)
  } else {
    if (refreshInterval) {
      clearInterval(refreshInterval)
      refreshInterval = null
    }
  }
  
  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval)
  })
</script>

{#if show}
<div class="bg-gray-800 rounded-lg p-4 mb-4">
  <h3 class="text-lg font-semibold mb-3">ストリーム選択</h3>
  
  {#if isLoading && !streamInfo}
    <p class="text-gray-400">読み込み中...</p>
  {:else if error}
    <p class="text-red-400">{error}</p>
  {:else if streamInfo && streamInfo.stream_info}
    <div class="space-y-4">
      <!-- Video streams -->
      <div>
        <label for="video-stream-select" class="block text-sm font-medium text-gray-300 mb-2">映像ストリーム:</label>
        <select 
          id="video-stream-select"
          bind:value={selectedVideoStream}
          class="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:border-blue-500"
          disabled={isLoading}
        >
          <option value={-1}>自動選択</option>
          {#each getStreamsByType(streamInfo.stream_info.streams, 'video') as stream}
            <option value={stream.index}>
              {getStreamDescription(stream)}
            </option>
          {/each}
        </select>
      </div>
      
      <!-- Audio streams -->
      <div>
        <label for="audio-stream-select" class="block text-sm font-medium text-gray-300 mb-2">音声ストリーム:</label>
        <select 
          id="audio-stream-select"
          bind:value={selectedAudioStream}
          class="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:border-blue-500"
          disabled={isLoading}
        >
          <option value={-1}>自動選択</option>
          {#each getStreamsByType(streamInfo.stream_info.streams, 'audio') as stream}
            <option value={stream.index}>
              {getStreamDescription(stream)}
            </option>
          {/each}
        </select>
      </div>
      
      <!-- Apply button -->
      <div class="flex justify-end mt-4">
        <button
          on:click={applyStreamSelection}
          disabled={isLoading}
          class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:bg-gray-600 disabled:cursor-not-allowed transition-colors"
        >
          {isLoading ? '適用中...' : 'ストリームを切り替え'}
        </button>
      </div>
      
      <!-- Current status -->
      {#if streamInfo.video_stream_index >= 0 || streamInfo.audio_stream_index >= 0}
        <div class="text-sm text-gray-400 mt-2">
          現在の選択: 映像 #{streamInfo.video_stream_index}, 音声 #{streamInfo.audio_stream_index}
        </div>
      {/if}
    </div>
  {:else if streamInfo}
    <p class="text-gray-400">ストリーム情報を取得できません</p>
  {:else}
    <p class="text-gray-400">ストリームが開始されていません</p>
  {/if}
</div>
{/if}