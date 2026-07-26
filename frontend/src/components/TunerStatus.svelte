<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { tunerDetails, type TunerInfo } from '../lib/tunerStatus'

  let tuners: TunerInfo[] = []
  let interval: ReturnType<typeof setInterval> | null = null
  let fetchingTuners = false
  let tunerError = false

  async function fetchTuners() {
    if (fetchingTuners) return
    fetchingTuners = true
    try {
      const res = await fetch('/api/tuners')
      if (res.ok) {
        tuners = await res.json()
        tunerError = false
        // Sort: GR first, then BS/CS
        tuners.sort((a, b) => {
          if (a.type === 'GR') return -1
          if (b.type === 'GR') return 1
          return a.type.localeCompare(b.type)
        })
      } else {
        tuners = []
        tunerError = true
      }
    } catch (e) {
      tuners = []
      tunerError = true
    } finally {
      fetchingTuners = false
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
    <span title={tunerDetails(t)}>
      <span class="text-gray-500">{t.type}:</span>
      <span class="{t.viewerUsing > 0 ? 'text-green-400' : 'text-gray-500'}">視聴 {t.viewerUsing}</span>
      <span class="text-gray-600"> · </span>
      <span class="{t.using >= t.total ? 'text-yellow-400' : 'text-gray-400'}">全体 {t.using}/{t.total}</span>
    </span>
  {/each}
  {#if tunerError}
    <span class="text-red-400">チューナー取得不能</span>
  {:else if tuners.length === 0}
    <span class="text-gray-600">--</span>
  {/if}
</div>
