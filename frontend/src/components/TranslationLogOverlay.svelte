<script lang="ts">
  import { afterUpdate } from 'svelte'
  import type { TranslationLogState, TranslationUIStatus } from '../lib/translationLog'

  export let channelId = ''
  export let log: TranslationLogState
  export let status: TranslationUIStatus

  let scroller: HTMLDivElement
  let followingLatest = true
  let unseenEntries = 0
  let previousChannelId = ''
  let previousEntryCount = 0

  const timeFormatter = new Intl.DateTimeFormat('ja-JP', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })

  function statusLabel(value: TranslationUIStatus): string {
    switch (value) {
      case 'connecting': return '接続中'
      case 'ready': return '翻訳中'
      case 'error': return 'エラー'
      default: return '停止中'
    }
  }

  function scrollToLatest() {
    if (!scroller) return
    scroller.scrollTop = scroller.scrollHeight
    followingLatest = true
    unseenEntries = 0
  }

  function handleScroll() {
    if (!scroller) return
    followingLatest = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= 48
    if (followingLatest) unseenEntries = 0
  }

  afterUpdate(() => {
    if (!scroller) return
    if (channelId !== previousChannelId) {
      previousChannelId = channelId
      previousEntryCount = log.entries.length
      scrollToLatest()
      return
    }
    const addedEntries = Math.max(0, log.entries.length - previousEntryCount)
    if (followingLatest) scrollToLatest()
    else if (addedEntries) unseenEntries += addedEntries
    previousEntryCount = log.entries.length
  })
</script>

<aside
  class="translation-log-overlay"
  aria-label="翻訳ログ"
>
  <header class="translation-log-header">
    <div>
      <strong>翻訳ログ</strong>
      <span class="translation-log-credit">VOICEVOX:ずんだもん</span>
    </div>
    <span class:error={status === 'error'} class="translation-log-status">
      <i class:ready={status === 'ready'}></i>{statusLabel(status)}
    </span>
  </header>

  <div class="translation-log-scroll" bind:this={scroller} on:scroll={handleScroll} role="log" aria-live="polite" aria-relevant="additions">
    {#if !log.entries.length && !log.draft}
      <p class="translation-log-empty">翻訳された発話がここに追加されます</p>
    {/if}
    {#each log.entries as entry (entry.id)}
      <article class="translation-log-entry">
        <time datetime={new Date(entry.receivedAt).toISOString()}>{timeFormatter.format(entry.receivedAt)}</time>
        {#if entry.originalText}<p class="translation-log-original">{entry.originalText}</p>{/if}
        <p class="translation-log-text">{entry.translatedText}</p>
      </article>
    {/each}
    {#if log.draft}
      <article class="translation-log-entry draft" aria-hidden="true">
        {#if log.draft.originalText}<p class="translation-log-original">{log.draft.originalText}</p>{/if}
        <p class="translation-log-text">{log.draft.translatedText}</p>
      </article>
    {/if}
  </div>

  {#if !followingLatest}
    <button class="translation-log-latest" type="button" on:click|stopPropagation={scrollToLatest}>
      最新へ{unseenEntries ? ` (${unseenEntries})` : ''}
    </button>
  {/if}
</aside>
