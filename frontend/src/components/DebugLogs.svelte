<script lang="ts">
  export let debugLogs: string[] = []
  export let pipelineLogs: string[] = []
  export let selectedChannel: any = null
  export let streamStarted: boolean = false
</script>

<div class="bg-gray-800 rounded-lg p-2 h-full flex flex-col gap-2">
  <!-- Application Debug Logs -->
  <div class="flex-1 min-h-0 flex flex-col">
    <div class="flex justify-between items-center mb-1">
      <h3 class="text-sm font-semibold">アプリケーションログ</h3>
      <button 
        class="px-2 py-1 bg-red-600 text-white rounded text-xs hover:bg-red-700"
        on:click={() => debugLogs = []}
      >
        クリア
      </button>
    </div>
    <div class="bg-black rounded p-2 flex-1 overflow-y-auto">
      {#if debugLogs.length === 0}
        <p class="text-gray-400 text-xs">ログはありません</p>
      {:else}
        {#each debugLogs as log}
          <div class="text-xs font-mono mb-1 text-green-400">
            {log}
          </div>
        {/each}
      {/if}
    </div>
  </div>
  
  <!-- Native pipeline logs -->
  <div class="flex-1 min-h-0 flex flex-col">
    <div class="flex justify-between items-center mb-1">
      <h3 class="text-sm font-semibold">配信パイプラインログ</h3>
      <button 
        class="px-2 py-1 bg-red-600 text-white rounded text-xs hover:bg-red-700"
        on:click={() => pipelineLogs = []}
      >
        クリア
      </button>
    </div>
    <div class="bg-black rounded p-2 flex-1 overflow-y-auto">
      {#if pipelineLogs.length === 0}
        <p class="text-gray-400 text-xs">パイプラインログはありません</p>
      {:else}
        {#each pipelineLogs as log}
          <div class="text-xs font-mono mb-1 text-yellow-400 whitespace-pre-wrap">
            {log}
          </div>
        {/each}
      {/if}
    </div>
  </div>
  
  <!-- Status -->
  <div class="text-xs text-gray-400 px-2 py-1 border-t border-gray-700">
    チャンネル: {selectedChannel?.displayName || selectedChannel?.name || 'なし'} | 
    ストリーミング: {streamStarted ? '再生中' : '待機中'}
  </div>
</div>
