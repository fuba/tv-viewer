<script lang="ts">
  import { onDestroy, createEventDispatcher } from 'svelte'
  import ProgramDetailModal from './ProgramDetailModal.svelte'
  
  export let channel: any

  const dispatch = createEventDispatcher()
  
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
      // Use serviceId (small number) from services array - this is what Mirakurun programs API expects
      // Note: service.id is the large composite ID, service.serviceId is the actual service ID
      const serviceId = channel.services?.[0]?.serviceId
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

  function watchChannel() {
    dispatch('select', channel)
    closeModal()
  }
</script>

<section class="compact-guide">
  <h2>選択中のチャンネル</h2>
  
  {#if !channel}
    <p class="guide-message">チャンネルを選択してください</p>
  {:else if loading}
    <p class="guide-message">読み込み中...</p>
  {:else if !currentProgram && nextPrograms.length === 0}
    <p class="guide-message">番組情報がありません</p>
  {:else}
    <div class="guide-content">
      {#if currentProgram}
        <!-- Current Program -->
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <button class="current-program"
             on:click={() => openProgramDetail(currentProgram)}>
          <div class="program-meta">
            <span class="program-badge">放送中</span>
            <span>残り {getTimeRemaining(currentProgram)}</span>
          </div>
          <div class="program-title">{currentProgram.name}</div>
          <div class="program-meta">
            {formatTime(currentProgram.startAt)} - {formatTime(currentProgram.startAt + currentProgram.duration)}
            <span>({formatDuration(currentProgram.duration)})</span>
          </div>

          {#if currentProgram.description}
            <div class="program-desc line-clamp-2">
              {currentProgram.description}
            </div>
          {/if}

          <div class="progress-track">
            <div class="progress-value" style="width: {getProgress(currentProgram)}%"></div>
          </div>
        </button>
      {/if}
      
      {#if nextPrograms.length > 0}
        <!-- Next Programs -->
        <div class="upcoming-programs">
          <div class="upcoming-heading">
            {currentProgram ? `次の番組 (${nextPrograms.length}件)` : `今後の番組 (${nextPrograms.length}件)`}
          </div>
          <div class="upcoming-list">
            {#each nextPrograms as program, index}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <!-- svelte-ignore a11y-no-static-element-interactions -->
              <button class="upcoming-program"
                   on:click={() => openProgramDetail(program)}>
                <div class="program-row">
                  <div class="program-title">{program.name}</div>
                  <div class="program-index">#{index + 1}</div>
                </div>
                <div class="program-meta">
                  {formatTime(program.startAt)} - {formatTime(program.startAt + program.duration)}
                  <span>({formatDuration(program.duration)})</span>
                </div>
                {#if program.description}
                  <div class="program-desc line-clamp-1">
                    {program.description}
                  </div>
                {/if}
              </button>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</section>

<!-- Program Detail Modal -->
<ProgramDetailModal 
  program={selectedProgram}
  {channel}
  isOpen={isModalOpen}
  on:close={closeModal}
  on:watch={watchChannel}
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

  .compact-guide { min-height: 8rem; }
  .compact-guide > h2 { margin-bottom: .7rem; color: var(--muted); font-size: .72rem; font-weight: 650; letter-spacing: .06em; }
  .guide-message { padding: 1rem 0; color: var(--muted); font-size: .85rem; }
  .guide-content { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr); gap: .75rem; }
  .current-program, .upcoming-program { width: 100%; border: 1px solid var(--line); background: var(--surface); text-align: left; }
  .current-program { padding: .9rem; border-color: var(--line-strong); }
  .current-program:hover, .upcoming-program:hover { border-color: var(--line-strong); background: var(--surface-hover); }
  .upcoming-programs { min-width: 0; }
  .upcoming-heading { position: sticky; top: 0; z-index: 1; padding: .3rem 0 .45rem; background: var(--canvas); color: var(--muted); font-size: .72rem; font-weight: 600; }
  .upcoming-list { display: grid; gap: 1px; max-height: 18rem; overflow-y: auto; }
  .upcoming-program { padding: .65rem .7rem; }
  .program-row { display: flex; align-items: flex-start; justify-content: space-between; gap: .5rem; }
  .program-title { color: var(--text); font-size: .82rem; font-weight: 600; line-height: 1.35; }
  .program-index { flex: 0 0 auto; color: var(--muted); font-size: .68rem; }
  .program-meta { display: flex; align-items: center; justify-content: space-between; gap: .5rem; margin: .3rem 0; color: var(--muted); font-size: .7rem; }
  .program-badge { color: var(--text); font-weight: 650; }
  .program-desc { margin-bottom: .45rem; color: var(--text-secondary); font-size: .72rem; line-height: 1.5; }
  .progress-track { height: 2px; overflow: hidden; background: var(--surface-active); }
  .progress-value { height: 100%; background: var(--accent); transition: width 1s linear; }
  @media (max-width: 640px) {
    .guide-content { grid-template-columns: 1fr; }
    .upcoming-list { max-height: 16rem; }
  }
</style>
