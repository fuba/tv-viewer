<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import ProgramDetailModal from './ProgramDetailModal.svelte'
  
  export let channel: any
  
  let programs: any[] = []
  let loading = false
  let currentProgram: any = null
  let nextPrograms: any[] = []
  let updateTimer: NodeJS.Timeout | null = null
  let selectedProgram: any = null
  let isModalOpen: boolean = false
  
  $: if (channel) {
    fetchPrograms(channel)
  }
  
  onDestroy(() => {
    if (updateTimer) {
      clearInterval(updateTimer)
    }
  })
  
  async function fetchPrograms(channel: any) {
    loading = true
    try {
      // Use serviceId if available (for CS services), otherwise use first service ID
      const serviceId = channel.serviceId || channel.services?.[0]?.id
      if (!serviceId) {
        console.log('No serviceId found for channel:', channel)
        programs = []
        currentProgram = null
        nextPrograms = []
        return
      }
      console.log(`Fetching programs for serviceId: ${serviceId}`)
      const response = await fetch(`/api/programs?serviceId=${serviceId}`)
      if (!response.ok) {
        console.error('API error:', response.status, await response.text())
        programs = []
        currentProgram = null
        nextPrograms = []
        return
      }
      const data = await response.json()
      console.log(`Received ${data.length} programs`)
      programs = Array.isArray(data) ? data : []
      
      updateProgramDisplay()
      
      // Update every minute to keep current program status
      if (updateTimer) clearInterval(updateTimer)
      updateTimer = setInterval(updateProgramDisplay, 60000)
      
    } catch (error) {
      console.error('Failed to fetch programs:', error)
      programs = []
      currentProgram = null
      nextPrograms = []
    } finally {
      loading = false
    }
  }
  
  function updateProgramDisplay() {
    const now = Date.now()
    
    // Find current program
    currentProgram = programs.find(p => 
      p.startAt <= now && p.startAt + p.duration > now
    )
    
    // Find next programs
    if (currentProgram) {
      const currentEndTime = currentProgram.startAt + currentProgram.duration
      nextPrograms = programs
        .filter(p => p.startAt >= currentEndTime)
        .slice(0, 10)
    } else {
      // If no current program, show next 10 programs
      nextPrograms = programs
        .filter(p => p.startAt > now)
        .slice(0, 10)
    }
  }
  
  function formatTime(timestamp: number) {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })
  }
  
  function formatDuration(duration: number) {
    const minutes = Math.floor(duration / 60000)
    const hours = Math.floor(minutes / 60)
    const mins = minutes % 60
    return hours > 0 ? `${hours}時間${mins}分` : `${mins}分`
  }
  
  function getProgress(program: any) {
    const now = Date.now()
    const elapsed = now - program.startAt
    return Math.min(100, Math.max(0, (elapsed / program.duration) * 100))
  }
  
  function getTimeRemaining(program: any) {
    const now = Date.now()
    const remaining = program.startAt + program.duration - now
    return formatDuration(remaining)
  }
  
  function openProgramDetail(program: any) {
    selectedProgram = program
    isModalOpen = true
  }
  
  function closeModal() {
    isModalOpen = false
    selectedProgram = null
  }
</script>

<div class="bg-gray-800 rounded-lg p-3 h-full flex flex-col">
  <h2 class="text-sm font-semibold mb-3 px-1">番組表</h2>
  
  {#if !channel}
    <p class="text-gray-400 text-sm px-1">チャンネルを選択してください</p>
  {:else if loading}
    <p class="text-gray-400 text-sm px-1">読み込み中...</p>
  {:else if !currentProgram && nextPrograms.length === 0}
    <p class="text-gray-400 text-sm px-1">番組情報がありません</p>
  {:else}
    <div class="flex-1 overflow-y-auto space-y-3">
      {#if currentProgram}
        <!-- Current Program -->
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div class="bg-gray-900 rounded-lg p-3 border border-blue-500 cursor-pointer hover:bg-gray-800 transition-colors"
             on:click={() => openProgramDetail(currentProgram)}>
          <div class="flex items-center justify-between mb-1">
            <span class="text-xs font-medium text-blue-400">現在放送中</span>
            <span class="text-xs text-gray-400">残り {getTimeRemaining(currentProgram)}</span>
          </div>
          <div class="text-sm font-medium mb-1 text-white">{currentProgram.name}</div>
          <div class="text-xs text-gray-400 mb-2">
            {formatTime(currentProgram.startAt)} - {formatTime(currentProgram.startAt + currentProgram.duration)}
            <span class="text-gray-500">({formatDuration(currentProgram.duration)})</span>
          </div>
          
          {#if currentProgram.description}
            <div class="text-xs text-gray-300 mb-2 line-clamp-2">
              {currentProgram.description}
            </div>
          {/if}
          
          <!-- Progress Bar -->
          <div class="w-full bg-gray-700 rounded-full h-1.5">
            <div class="bg-blue-500 h-1.5 rounded-full transition-all duration-1000" 
                 style="width: {getProgress(currentProgram)}%"></div>
          </div>
          
          <!-- Click hint -->
          <div class="text-xs text-gray-500 mt-1 text-center">タップで詳細表示</div>
        </div>
      {/if}
      
      {#if nextPrograms.length > 0}
        <!-- Next Programs -->
        <div class="space-y-1.5">
          <div class="text-xs font-medium text-gray-400 px-1 sticky top-0 bg-gray-800 py-1">
            {currentProgram ? `次の番組 (${nextPrograms.length}件)` : `今後の番組 (${nextPrograms.length}件)`}
          </div>
          <div class="space-y-1.5 max-h-80 overflow-y-auto">
            {#each nextPrograms as program, index}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <!-- svelte-ignore a11y-no-static-element-interactions -->
              <div class="bg-gray-900 rounded-lg p-2 border border-gray-700 hover:border-gray-600 transition-colors cursor-pointer"
                   on:click={() => openProgramDetail(program)}>
                <div class="flex items-start justify-between mb-1">
                  <div class="text-sm font-medium text-gray-200 flex-1 pr-2">{program.name}</div>
                  <div class="text-xs text-gray-500 flex-shrink-0">#{index + 1}</div>
                </div>
                <div class="text-xs text-gray-400 mb-1">
                  {formatTime(program.startAt)} - {formatTime(program.startAt + program.duration)}
                  <span class="text-gray-500">({formatDuration(program.duration)})</span>
                </div>
                {#if program.description}
                  <div class="text-xs text-gray-400 line-clamp-1">
                    {program.description}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<!-- Program Detail Modal -->
<ProgramDetailModal 
  program={selectedProgram}
  isOpen={isModalOpen}
  on:close={closeModal}
/>

<style>
  .line-clamp-1 {
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 1;
    -webkit-box-orient: vertical;
  }
  
  .line-clamp-2 {
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
</style>