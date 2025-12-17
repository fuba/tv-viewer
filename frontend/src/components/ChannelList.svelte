<script lang="ts">
  import { onMount } from 'svelte'
  
  export let selectedChannel: any = null
  
  let channels: any[] = []
  let loading = true
  let activeTab: 'GR' | 'BS' | 'CS' = 'GR'
  
  onMount(async () => {
    try {
      const response = await fetch('/api/channels')
      const allChannels = await response.json()
      
      // Process channels: For CS channels, expand services as individual items
      const processedChannels: any[] = []
      
      allChannels.forEach((ch: any) => {
        if (!ch.services || ch.services.length === 0) return
        
        if (ch.type === 'CS') {
          // For CS channels, create an item for each service
          ch.services.forEach((service: any) => {
            processedChannels.push({
              ...ch,
              serviceId: service.serviceId || service.id,
              serviceName: service.name,
              displayName: service.name,
              // Keep original channel info for API calls
              originalChannel: ch.channel,
              isService: true
            })
          })
        } else if (ch.type === 'GR' || ch.type === 'BS') {
          // For GR and BS, keep as is
          processedChannels.push({
            ...ch,
            displayName: ch.services[0]?.name || ch.name,
            serviceId: ch.services[0]?.serviceId || ch.services[0]?.id,
            isService: false
          })
        }
      })
      
      // Sort channels by type and service name for CS
      channels = processedChannels.sort((a: any, b: any) => {
        // First sort by type (GR first, then BS, then CS)
        if (a.type !== b.type) {
          const typeOrder = { 'GR': 0, 'BS': 1, 'CS': 2 }
          return (typeOrder[a.type as keyof typeof typeOrder] || 9) - (typeOrder[b.type as keyof typeof typeOrder] || 9)
        }
        
        // For CS channels, sort by service name
        if (a.type === 'CS') {
          return a.displayName.localeCompare(b.displayName)
        }
        
        // For others, sort by channel number
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

<div class="bg-gray-800 rounded-lg p-2 h-full flex flex-col">
  <h2 class="text-lg font-semibold mb-2 px-2">チャンネル一覧</h2>
  
  {#if loading}
    <p class="text-gray-400 px-2">読み込み中...</p>
  {:else if channels.length === 0}
    <p class="text-gray-400 px-2">チャンネルがありません</p>
  {:else}
    <!-- Tab navigation -->
    <div class="flex border-b border-gray-700 mb-2">
      <button 
        class="px-3 py-1 text-sm font-medium transition-colors
               {activeTab === 'GR' ? 'text-green-400 border-b-2 border-green-400' : 'text-gray-400 hover:text-green-300'}"
        on:click={() => activeTab = 'GR'}
      >
        地上波
      </button>
      <button 
        class="px-3 py-1 text-sm font-medium transition-colors
               {activeTab === 'BS' ? 'text-blue-400 border-b-2 border-blue-400' : 'text-gray-400 hover:text-blue-300'}"
        on:click={() => activeTab = 'BS'}
      >
        BS
      </button>
      <button 
        class="px-3 py-1 text-sm font-medium transition-colors
               {activeTab === 'CS' ? 'text-purple-400 border-b-2 border-purple-400' : 'text-gray-400 hover:text-purple-300'}"
        on:click={() => activeTab = 'CS'}
      >
        CS ({channels.filter(ch => ch.type === 'CS').length})
      </button>
    </div>
    
    <!-- Scrollable channel list -->
    <div class="flex-1 overflow-y-auto">
      <div class="grid {activeTab === 'CS' ? 'grid-cols-1 sm:grid-cols-2' : 'grid-cols-2 sm:grid-cols-3'} gap-1 p-1">
        {#each channels.filter(ch => ch.type === activeTab) as channel}
          <button
            class="text-left p-2 rounded text-sm hover:bg-gray-700 transition-colors
                   {selectedChannel?.channel === channel.channel && 
                    selectedChannel?.serviceId === channel.serviceId ? 'bg-gray-700' : ''}"
            on:click={() => selectChannel(channel)}
          >
            <div class="font-medium truncate">{channel.displayName}</div>
            <div class="text-xs text-gray-400 truncate">
              {#if channel.type === 'CS' && channel.isService}
                Ch.{channel.channel} - ID:{channel.serviceId}
              {:else}
                Ch.{channel.channel}
              {/if}
            </div>
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>