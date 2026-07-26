<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { createEventDispatcher } from 'svelte'
  import { tunerDetails, type TunerInfo } from '../lib/tunerStatus'

  export let channel: any = null
  export let connectionStatus: string = 'disconnected'

  const dispatch = createEventDispatcher()

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

  // Get status color
  $: statusColor = connectionStatus === 'connected' ? 'bg-green-500' :
                   connectionStatus === 'connecting' ? 'bg-yellow-500' :
                   connectionStatus === 'failed' ? 'bg-red-500' :
                   'bg-gray-500'
</script>

<div class="meta-bar">
  <div class="meta-inner">
    <button
      class="meta-channel"
      on:click={() => dispatch('channelClick')}
    >
      <span class="meta-status {statusColor}"></span>
      <span class="meta-channel-copy">
        <strong>{channel?.displayName || 'チャンネルを選択'}</strong>
        <small>Ch.{channel?.channel || '—'} · {connectionStatus === 'connected' ? 'ライブ' : '待機中'}</small>
      </span>
    </button>

    <div class="tuner-summary">
      {#each tuners as t}
        <span class="tuner-pill" class:viewer-active={t.viewerUsing > 0} title={tunerDetails(t)}>
          <b>{t.type}</b> {t.using}/{t.total}
        </span>
      {/each}
      {#if tunerError}
        <span class="tuner-error">取得不能</span>
      {:else if tuners.length === 0}
        <span class="tuner-empty">—</span>
      {/if}
    </div>

    <button
      class="meta-settings"
      on:click={() => dispatch('settingsClick')}
      aria-label="設定を開く"
    >
      <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
    </button>
  </div>
</div>
