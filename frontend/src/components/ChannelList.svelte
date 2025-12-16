<script lang="ts">
  import { onMount } from 'svelte'
  
  export let selectedChannel: any = null
  
  let channels: any[] = []
  let loading = true
  
  onMount(async () => {
    try {
      const response = await fetch('/api/channels')
      const allChannels = await response.json()
      // Filter out channels without services and disable CS channels (segfaults)
      // Enable GR (terrestrial) and BS (satellite) channels
      channels = allChannels.filter((ch: any) => 
        ch.services && 
        ch.services.length > 0 && 
        (ch.type === 'GR' || ch.type === 'BS') // Enable terrestrial and BS satellite channels
      )
      
      // Sort channels by type and channel number for better organization
      channels.sort((a: any, b: any) => {
        // First sort by type (GR first, then BS)
        if (a.type !== b.type) {
          return a.type === 'GR' ? -1 : 1
        }
        // Then sort by channel number within same type
        return a.channel.localeCompare(b.channel, undefined, { numeric: true })
      })
    } catch (error) {
      console.error('Failed to fetch channels:', error)
    } finally {
      loading = false
    }
  })
  
  function selectChannel(channel: any) {
    selectedChannel = channel
  }
</script>

<div class="bg-gray-800 rounded-lg p-4">
  <h2 class="text-xl font-semibold mb-4">チャンネル一覧</h2>
  
  {#if loading}
    <p class="text-gray-400">読み込み中...</p>
  {:else if channels.length === 0}
    <p class="text-gray-400">チャンネルがありません</p>
  {:else}
    <div class="space-y-2">
      {#each channels as channel}
        <button
          class="w-full text-left p-3 rounded hover:bg-gray-700 transition-colors
                 {selectedChannel?.channel === channel.channel ? 'bg-gray-700' : ''}"
          on:click={() => selectChannel(channel)}
        >
          <div class="font-medium">
            {#if channel.services && channel.services.length > 0}
              {channel.services[0].name}
            {:else}
              {channel.name}
            {/if}
          </div>
          <div class="text-sm text-gray-400">
            {#if channel.type === 'GR'}
              <span class="text-green-400">地上波</span>
            {:else if channel.type === 'BS'}
              <span class="text-blue-400">BS</span>
            {:else}
              <span class="text-yellow-400">{channel.type}</span>
            {/if}
            Ch. {channel.channel}
          </div>
        </button>
      {/each}
    </div>
  {/if}
</div>