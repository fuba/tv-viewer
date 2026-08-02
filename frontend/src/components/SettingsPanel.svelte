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
        <p class="log-empty">ログはありません</p>
      {:else}
        {#each debugLogs as log}
          <div class="log-line">
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
        <p class="log-empty">パイプラインログはありません</p>
      {:else}
        {#each pipelineLogs as log}
          <div class="log-line is-pipeline">
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
  .diagnostic-card { padding: .8rem; border: 1px solid var(--line); background: var(--surface); }
  .diagnostic-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .6rem; }
  .diagnostic-header h3 { color: var(--text-secondary); font-size: .78rem; font-weight: 650; }
  .clear-button { min-height: 2rem; padding: 0 .6rem; border-radius: .25rem; color: var(--muted); font-size: .7rem; }
  .clear-button:hover { background: var(--surface-hover); color: var(--text); }
  .log-view { height: 9rem; overflow-y: auto; padding: .6rem; background: #000; }
  .log-empty { color: var(--muted); font-size: .7rem; }
  .log-line { margin-bottom: .2rem; color: var(--text-secondary); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: .68rem; }
  .log-line.is-pipeline { color: var(--muted); white-space: pre-wrap; }
  .diagnostic-status { padding: .5rem .1rem 0; color: var(--muted); font-size: .72rem; }
</style>
