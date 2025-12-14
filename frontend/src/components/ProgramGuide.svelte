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
      const response = await fetch(`/api/programs?serviceId=${channel.services?.[0]?.id}`)
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

<div class="bg-gray-800 rounded-lg p-4">
  <h2 class="text-xl font-semibold mb-4">番組表</h2>
  
  {#if !channel}
    <p class="text-gray-400">チャンネルを選択してください</p>
  {:else if loading}
    <p class="text-gray-400">読み込み中...</p>
  {:else if programs.length === 0}
    <p class="text-gray-400">番組情報がありません</p>
  {:else}
    <div class="space-y-3">
      {#each programs as program}
        <div class="border-l-4 border-blue-500 pl-3">
          <div class="font-medium">{program.name}</div>
          <div class="text-sm text-gray-400">
            {formatTime(program.startAt)} - {formatTime(program.startAt + program.duration)}
          </div>
          {#if program.description}
            <div class="text-sm text-gray-300 mt-1">{program.description}</div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>