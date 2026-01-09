<script lang="ts">
  import { onMount, onDestroy } from 'svelte'

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
</script>

<div class="fixed bottom-0 left-0 right-0 bg-gray-900/90 text-xs text-gray-400 px-2 py-1 flex justify-center gap-4 z-50">
  {#each tuners as t}
    <span>
      <span class="text-gray-500">{t.type}:</span>
      <span class="{t.using > 0 ? 'text-yellow-400' : 'text-gray-500'}">{t.using}</span><span class="text-gray-600">/{t.total}</span>
    </span>
  {/each}
  {#if tuners.length === 0}
    <span class="text-gray-600">--</span>
  {/if}
</div>
