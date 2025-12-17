<script lang="ts">
  import { onMount } from 'svelte'
  
  export let channel: any
  
  let programs: any[] = []
  let loading = false
  
  $: if (channel) {
    fetchPrograms(channel)
  }
  
  async function fetchPrograms(channel: any) {
    loading = true
    try {
      // Use serviceId if available (for CS services), otherwise use first service ID
      const serviceId = channel.serviceId || channel.services?.[0]?.id
      if (!serviceId) {
        programs = []
        return
      }
      const response = await fetch(`/api/programs?serviceId=${serviceId}`)
      programs = await response.json()
    } catch (error) {
      console.error('Failed to fetch programs:', error)
      programs = []
    } finally {
      loading = false
    }
  }
  
  function formatTime(timestamp: number) {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })
  }
</script>

<div class="bg-gray-800 rounded-lg p-2 h-full flex flex-col">
  <h2 class="text-sm font-semibold mb-2 px-2">番組表</h2>
  
  {#if !channel}
    <p class="text-gray-400 text-sm px-2">チャンネルを選択してください</p>
  {:else if loading}
    <p class="text-gray-400 text-sm px-2">読み込み中...</p>
  {:else if programs.length === 0}
    <p class="text-gray-400 text-sm px-2">番組情報がありません</p>
  {:else}
    <div class="flex-1 overflow-y-auto px-2">
      <div class="space-y-1 lg:space-y-2">
        {#each programs.slice(0, 3) as program}
          <div class="border-l-2 border-blue-500 pl-2 py-1">
            <div class="text-xs lg:text-sm font-medium truncate">{program.name}</div>
            <div class="text-xs text-gray-400 hidden lg:block">
              {formatTime(program.startAt)} - {formatTime(program.startAt + program.duration)}
            </div>
            <div class="text-xs text-gray-400 lg:hidden">
              {formatTime(program.startAt)}
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>