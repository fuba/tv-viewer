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

<div class="bar-tuners">
  {#each tuners as t}
    <span class:tuner-live={t.viewerUsing > 0} title={tunerDetails(t)}>
      <b>{t.type}</b> {t.using}/{t.total}
    </span>
  {/each}
  {#if tunerError}
    <span class="tuner-error">チューナー取得不能</span>
  {/if}
</div>
