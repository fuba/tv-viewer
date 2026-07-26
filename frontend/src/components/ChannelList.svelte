<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte'
  import { channelMatchesSelection, expandChannelServices } from '../lib/channelSelection'

  export let selectedChannel: any = null
  export let activeChannelId = ''
  export let activeViewerCount = 0

  const dispatch = createEventDispatcher()

  let channels: any[] = []
  let loading = true
  let activeTab: 'GR' | 'BS' | 'CS' = 'GR'
  
  onMount(async () => {
    try {
      const response = await fetch('/api/channels')
      const allChannels = await response.json()
      
      const processedChannels = expandChannelServices(allChannels)
      
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
    dispatch('select', channel)
  }
</script>

<section class="channel-picker">
  
  {#if loading}
    <p class="channel-message">読み込み中...</p>
  {:else if channels.length === 0}
    <p class="channel-message">チャンネルがありません</p>
  {:else}
    <!-- Tab navigation -->
    <div class="channel-tabs" role="tablist" aria-label="放送種別">
      <button 
        class:active={activeTab === 'GR'}
        on:click={() => activeTab = 'GR'}
      >
        地上波
      </button>
      <button 
        class:active={activeTab === 'BS'}
        on:click={() => activeTab = 'BS'}
      >
        BS
      </button>
      <button 
        class:active={activeTab === 'CS'}
        on:click={() => activeTab = 'CS'}
      >
        CS ({channels.filter(ch => ch.type === 'CS').length})
      </button>
    </div>
    
    <!-- Scrollable channel list -->
    <div class="channel-grid">
        {#each channels.filter(ch => ch.type === activeTab) as channel}
          {@const locked = activeViewerCount > 1 && !channelMatchesSelection(channel, activeChannelId)}
          <button
            class="channel-card"
            class:locked
            class:selected={selectedChannel?.channel === channel.channel && selectedChannel?.serviceId === channel.serviceId}
            disabled={locked}
            title={locked ? 'ほかの視聴者がいるためチャンネルを変更できません' : ''}
            on:click={() => selectChannel(channel)}
          >
            <div class="channel-name">{channel.displayName}</div>
            <div class="channel-number">
              {#if channel.type === 'CS' && channel.isService}
                Ch.{channel.channel} - ID:{channel.serviceId}
              {:else}
                Ch.{channel.channel}
              {/if}
            </div>
          </button>
        {/each}
    </div>
  {/if}
</section>

<style>
  .channel-picker { min-height: 12rem; }
  .channel-message { padding: 1rem; color: var(--muted); text-align: center; }
  .channel-tabs { display: inline-flex; gap: .25rem; padding: .25rem; margin-bottom: 1rem; border-radius: .75rem; background: var(--surface-raised); }
  .channel-tabs button { min-height: 2.5rem; padding: 0 1rem; border-radius: .55rem; color: var(--muted); font-size: .875rem; font-weight: 650; }
  .channel-tabs button.active { background: var(--surface-active); color: var(--text); box-shadow: 0 1px 8px rgb(0 0 0 / 18%); }
  .channel-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr)); gap: .55rem; max-height: min(48vh, 30rem); overflow-y: auto; padding: .1rem; }
  .channel-card { min-height: 4.5rem; padding: .85rem; border: 1px solid var(--line); border-radius: .8rem; background: var(--surface); text-align: left; transition: border-color .15s, background .15s, transform .15s; }
  .channel-card:hover:not(:disabled) { border-color: var(--line-strong); background: var(--surface-hover); transform: translateY(-1px); }
  .channel-card.selected { border-color: var(--accent); background: var(--accent-soft); }
  .channel-card.locked { opacity: .38; cursor: not-allowed; }
  .channel-name { color: var(--text); font-size: .9rem; font-weight: 650; line-height: 1.25; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .channel-number { margin-top: .4rem; color: var(--muted); font-size: .72rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  @media (max-width: 640px) {
    .channel-tabs { display: flex; }
    .channel-tabs button { flex: 1; padding: 0 .7rem; }
    .channel-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); max-height: 52vh; }
    .channel-card { min-height: 4rem; padding: .7rem; }
  }
</style>
