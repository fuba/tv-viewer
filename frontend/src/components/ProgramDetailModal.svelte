<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  
  export let program: any = null
  export let isOpen: boolean = false
  
  const dispatch = createEventDispatcher()
  
  function close() {
    dispatch('close')
  }
  
  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      close()
    }
  }
  
  function formatTime(timestamp: number) {
    const date = new Date(timestamp)
    return date.toLocaleString('ja-JP', { 
      month: 'numeric',
      day: 'numeric',
      weekday: 'short',
      hour: '2-digit', 
      minute: '2-digit' 
    })
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
  
  function getGenreText(genre: any) {
    if (!genre || typeof genre.lv1 === 'undefined') return ''
    // 簡易的なジャンル表示（実際のジャンルコードに応じて拡張可能）
    const genres = [
      'ニュース/報道', 'スポーツ', '情報/ワイドショー', 'ドラマ', 
      '音楽', 'バラエティ', '映画', 'アニメ/特撮',
      '教養/ドキュメンタリー', '劇場/公演', '趣味/教育', '福祉'
    ]
    return genres[genre.lv1] || ''
  }
</script>

{#if isOpen && program}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div class="modal-backdrop" on:click={handleBackdropClick}>
    <div class="modal-content">
      <!-- Header -->
      <div class="modal-header">
        <h2 class="text-3xl font-bold text-white leading-tight">{program.name}</h2>
        <button 
          class="close-button"
          on:click={close}
          aria-label="閉じる"
        >
          ✕
        </button>
      </div>
      
      <!-- Body -->
      <div class="modal-body">
        <!-- Time and Duration -->
        <div class="info-section">
          <div class="info-row">
            <span class="info-label">放送時間</span>
            <span class="info-value">
              {formatTime(program.startAt)} ～ {formatTime(program.startAt + program.duration)}
            </span>
          </div>
          <div class="info-row">
            <span class="info-label">長さ</span>
            <span class="info-value">{formatDuration(program.duration)}</span>
          </div>
          {#if getGenreText(program.genre)}
            <div class="info-row">
              <span class="info-label">ジャンル</span>
              <span class="info-value">{getGenreText(program.genre)}</span>
            </div>
          {/if}
        </div>
        
        <!-- Progress for current program -->
        {#if program.startAt <= Date.now() && program.startAt + program.duration > Date.now()}
          <div class="progress-section">
            <div class="flex justify-between items-center mb-2">
              <span class="text-sm font-medium text-blue-400">現在放送中</span>
              <span class="text-sm text-gray-400">残り {getTimeRemaining(program)}</span>
            </div>
            <div class="w-full bg-gray-700 rounded-full h-2">
              <div class="bg-blue-500 h-2 rounded-full transition-all duration-1000" 
                   style="width: {getProgress(program)}%"></div>
            </div>
          </div>
        {/if}
        
        <!-- Description -->
        {#if program.description}
          <div class="description-section">
            <h3 class="text-sm font-semibold text-gray-400 mb-2">番組内容</h3>
            <p class="text-gray-200 whitespace-pre-wrap">{program.description}</p>
          </div>
        {/if}
        
        <!-- Additional Info -->
        {#if program.eventId || program.serviceId}
          <div class="additional-info">
            <div class="text-xs text-gray-500">
              {#if program.eventId}
                <span>イベントID: {program.eventId}</span>
              {/if}
              {#if program.serviceId}
                <span class="ml-4">サービスID: {program.serviceId}</span>
              {/if}
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.8);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    animation: fadeIn 0.2s ease-out;
  }
  
  .modal-content {
    background-color: #111827;
    width: 100%;
    height: 100%;
    overflow: hidden;
    animation: slideIn 0.3s ease-out;
    display: flex;
    flex-direction: column;
  }
  
  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 2rem;
    border-bottom: 1px solid #374151;
    flex-shrink: 0;
  }
  
  .close-button {
    background: none;
    border: none;
    color: #9ca3af;
    font-size: 1.5rem;
    cursor: pointer;
    padding: 0.25rem;
    line-height: 1;
    transition: color 0.2s;
  }
  
  .close-button:hover {
    color: #f3f4f6;
  }
  
  .modal-body {
    padding: 2rem;
    overflow-y: auto;
    flex-grow: 1;
    font-size: 1.125rem;
  }
  
  .info-section {
    margin-bottom: 1.5rem;
  }
  
  .info-row {
    display: flex;
    margin-bottom: 1rem;
    align-items: flex-start;
  }
  
  .info-label {
    flex-shrink: 0;
    width: 8rem;
    color: #9ca3af;
    font-size: 1.125rem;
    font-weight: 600;
  }
  
  .info-value {
    color: #e5e7eb;
    font-size: 1.125rem;
    line-height: 1.5;
  }
  
  .progress-section {
    background-color: #1f2937;
    padding: 1.5rem;
    border-radius: 0.75rem;
    margin-bottom: 2rem;
    border: 2px solid #374151;
  }
  
  .description-section {
    margin-bottom: 2rem;
  }
  
  .description-section h3 {
    font-size: 1.25rem;
    margin-bottom: 1rem;
  }
  
  .description-section p {
    font-size: 1.125rem;
    line-height: 1.7;
    color: #f3f4f6;
  }
  
  .additional-info {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid #374151;
  }
  
  @keyframes fadeIn {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
  
  @keyframes slideIn {
    from {
      transform: translateY(-20px);
      opacity: 0;
    }
    to {
      transform: translateY(0);
      opacity: 1;
    }
  }
  
  @media (max-width: 640px) {
    .modal-content {
      width: 95%;
      max-height: 90vh;
    }
    
    .modal-body {
      padding: 1rem;
    }
  }
</style>