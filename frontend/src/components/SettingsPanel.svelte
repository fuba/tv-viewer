<script lang="ts">
  export let selectedChannel: any = null
  export let debugLogs: string[] = []
  export let pipelineLogs: string[] = []
  export let streamStarted: boolean = false
</script>

<div class="diagnostics">
  <!-- Debug Logs Section -->
  <section class="diagnostic-card">
    <div class="diagnostic-header">
      <h3>アプリケーション</h3>
      <button
        class="clear-button"
        on:click={() => debugLogs = []}
      >
        クリア
      </button>
    </div>
    <div class="log-view">
      {#if debugLogs.length === 0}
        <p class="text-gray-500 text-xs">ログはありません</p>
      {:else}
        {#each debugLogs as log}
          <div class="text-xs font-mono mb-1 text-green-400">
            {log}
          </div>
        {/each}
      {/if}
    </div>
  </section>

  <!-- Native pipeline logs section -->
  <section class="diagnostic-card">
    <div class="diagnostic-header">
      <h3>配信パイプライン</h3>
      <button
        class="clear-button"
        on:click={() => pipelineLogs = []}
      >
        クリア
      </button>
    </div>
    <div class="log-view">
    {#if pipelineLogs.length === 0}
        <p class="text-gray-500 text-xs">パイプラインログはありません</p>
      {:else}
        {#each pipelineLogs as log}
          <div class="text-xs font-mono mb-1 text-yellow-400 whitespace-pre-wrap">
            {log}
          </div>
        {/each}
      {/if}
    </div>
  </section>

  <!-- Status -->
  <div class="diagnostic-status">
    チャンネル: {selectedChannel?.displayName || selectedChannel?.name || 'なし'} |
    ストリーミング: {streamStarted ? '再生中' : '待機中'}
  </div>
</div>

<style>
  .diagnostics { display: grid; gap: .75rem; }
  .diagnostic-card { padding: .85rem; border: 1px solid var(--line); border-radius: .8rem; background: var(--surface); }
  .diagnostic-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .65rem; }
  .diagnostic-header h3 { color: var(--text-secondary); font-size: .8rem; font-weight: 700; }
  .clear-button { min-height: 2rem; padding: 0 .6rem; border-radius: .45rem; color: #e98c94; font-size: .7rem; }
  .clear-button:hover { background: rgb(233 140 148 / 10%); }
  .log-view { height: 9rem; overflow-y: auto; padding: .7rem; border-radius: .55rem; background: #07090b; }
  .diagnostic-status { padding: .65rem .2rem 0; color: var(--muted); font-size: .72rem; }
</style>
