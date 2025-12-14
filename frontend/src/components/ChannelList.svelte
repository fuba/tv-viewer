<script lang="ts">
  import { onMount } from 'svelte'
  
  export let selectedChannel: any = null
  
  let channels: any[] = []
  let loading = true
  
  onMount(async () => {
    try {
      const response = await fetch('/api/channels')
      channels = await response.json()
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
                 {selectedChannel?.id === channel.id ? 'bg-gray-700' : ''}"
          on:click={() => selectChannel(channel)}
        >
          <div class="font-medium">{channel.name}</div>
          <div class="text-sm text-gray-400">Ch. {channel.channel}</div>
        </button>
      {/each}
    </div>
  {/if}
</div>