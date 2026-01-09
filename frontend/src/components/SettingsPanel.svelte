<script lang="ts">
  export let selectedChannel: any = null
  export let debugLogs: string[] = []
  export let ffmpegLogs: string[] = []
  export let streamStarted: boolean = false
</script>

<div class="space-y-4">
  <!-- Debug Logs Section -->
  <div class="bg-gray-900 rounded-lg p-3">
    <div class="flex justify-between items-center mb-2">
      <h3 class="text-sm font-semibold text-white">アプリケーションログ</h3>
      <button
        class="px-2 py-1 bg-red-600 text-white rounded text-xs hover:bg-red-700"
        on:click={() => debugLogs = []}
      >
        クリア
      </button>
    </div>
    <div class="bg-black rounded p-2 h-32 overflow-y-auto">
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
  </div>

  <!-- FFmpeg Logs Section -->
  <div class="bg-gray-900 rounded-lg p-3">
    <div class="flex justify-between items-center mb-2">
      <h3 class="text-sm font-semibold text-white">FFmpegログ</h3>
      <button
        class="px-2 py-1 bg-red-600 text-white rounded text-xs hover:bg-red-700"
        on:click={() => ffmpegLogs = []}
      >
        クリア
      </button>
    </div>
    <div class="bg-black rounded p-2 h-32 overflow-y-auto">
      {#if ffmpegLogs.length === 0}
        <p class="text-gray-500 text-xs">FFmpegログはありません</p>
      {:else}
        {#each ffmpegLogs as log}
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
