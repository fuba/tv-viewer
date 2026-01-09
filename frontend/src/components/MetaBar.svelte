<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { createEventDispatcher } from 'svelte'

  export let channel: any = null
  export let connectionStatus: string = 'disconnected'

  const dispatch = createEventDispatcher()

  interface TunerInfo {
    type: string
    total: number
    using: number
    free: number
  }

  let tuners: TunerInfo[] = []
  let interval: ReturnType<typeof setInterval> | null = null

  async function fetchTuners() {
    try {
      const res = await fetch('/api/tuners')
      if (res.ok) {
        tuners = await res.json()
        // Sort: GR first, then BS/CS
        tuners.sort((a, b) => {
          if (a.type === 'GR') return -1
          if (b.type === 'GR') return 1
          return a.type.localeCompare(b.type)
        })
      }
    } catch (e) {
      // Ignore errors
    }
  }

  onMount(() => {
    fetchTuners()
    interval = setInterval(fetchTuners, 3000)
  })

  onDestroy(() => {
    if (interval) clearInterval(interval)
  })

  // Get status color
  $: statusColor = connectionStatus === 'connected' ? 'bg-green-500' :
                   connectionStatus === 'connecting' ? 'bg-yellow-500' :
                   connectionStatus === 'failed' ? 'bg-red-500' :
                   'bg-gray-500'
</script>

<div class="fixed bottom-0 left-0 right-0 bg-gray-800/95 backdrop-blur border-t border-gray-700 z-30">
  <div class="max-w-[1800px] mx-auto px-2 sm:px-4 py-2 flex items-center justify-between">
    <!-- Left: Current channel info -->
    <button
      class="flex items-center gap-2 hover:bg-gray-700/50 px-2 sm:px-3 py-1 rounded min-w-0"
      on:click={() => dispatch('channelClick')}
    >
      <!-- Connection status indicator -->
      <span class="w-2 h-2 rounded-full flex-shrink-0 {statusColor}"></span>
      <!-- Channel name -->
      <span class="text-sm font-medium truncate max-w-[100px] sm:max-w-[200px]">
        {channel?.displayName || '未選択'}
      </span>
      <!-- Channel number (hidden on mobile) -->
      <span class="hidden sm:inline text-xs text-gray-400">
        Ch.{channel?.channel || '-'}
      </span>
    </button>

    <!-- Center: Tuner status -->
    <div class="flex items-center gap-2 sm:gap-4 text-xs text-gray-400">
      {#each tuners as t}
        <span class="flex items-center gap-1">
          <span class="text-gray-500">{t.type}:</span>
          <span class="{t.using > 0 ? 'text-yellow-400' : 'text-gray-500'}">{t.using}</span>
          <span class="text-gray-600">/{t.total}</span>
        </span>
      {/each}
      {#if tuners.length === 0}
        <span class="text-gray-600">--</span>
      {/if}
    </div>

    <!-- Right: Settings button -->
    <button
      class="p-2 hover:bg-gray-700/50 rounded text-gray-400 hover:text-white"
      on:click={() => dispatch('settingsClick')}
    >
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
    </button>
  </div>
</div>
