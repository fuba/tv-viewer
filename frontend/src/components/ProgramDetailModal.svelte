<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { reserveProgram } from '../lib/recording'
  
  export let program: any = null
  export let channel: any = null
  export let isOpen: boolean = false
  
  const dispatch = createEventDispatcher()
  let reservationState: 'idle' | 'submitting' | 'reserved' | 'error' = 'idle'
  let reservationError = ''
  let reservationProgramId: number | null = null
  let backdropPressed = false

  $: if ((program?.id ?? null) !== reservationProgramId) {
    reservationProgramId = program?.id ?? null
    reservationState = 'idle'
    reservationError = ''
  }
  
  function close() {
    dispatch('close')
  }

  function watchChannel() {
    if (!channel) return
    dispatch('watch', channel)
    close()
  }
  
  function handleBackdropPointerDown(e: PointerEvent) {
    backdropPressed = e.target === e.currentTarget
  }

  function handleBackdropClick(e: MouseEvent) {
    if (backdropPressed && e.target === e.currentTarget) {
      close()
    }
    backdropPressed = false
  }

  async function handleReserve() {
    if (!program || reservationState === 'submitting' || reservationState === 'reserved') return
    const programId = program.id
    reservationState = 'submitting'
    reservationError = ''
    try {
      await reserveProgram(programId)
      if (reservationProgramId !== programId) return
      reservationState = 'reserved'
    } catch (error) {
      if (reservationProgramId !== programId) return
      reservationState = 'error'
      reservationError = error instanceof Error ? error.message : '録画予約に失敗しました'
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
  <div class="modal-backdrop" on:pointerdown={handleBackdropPointerDown} on:click={handleBackdropClick}>
    <div class="modal-content">
      <!-- Header -->
      <div class="modal-header">
        <div>
          {#if channel}<p class="channel-eyebrow">{channel.displayName || channel.name}</p>{/if}
          <h2>{program.name}</h2>
        </div>
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
            <div class="progress-meta">
              <span class="progress-badge">放送中</span>
              <span>残り {getTimeRemaining(program)}</span>
            </div>
            <div class="progress-track">
              <div class="progress-value"
                   style="width: {getProgress(program)}%"></div>
            </div>
          </div>
        {/if}
        
        <!-- Description -->
        {#if program.description}
          <div class="description-section">
            <h3>番組内容</h3>
            <p class="whitespace-pre-wrap">{program.description}</p>
          </div>
        {/if}

        <div class="action-section">
          {#if channel}
            <button class="watch-button" on:click={watchChannel}>
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="M8 5v14l11-7z" />
              </svg>
              このチャンネルを見る
            </button>
          {/if}
          <button
            class="reservation-button"
            class:reserved={reservationState === 'reserved'}
            disabled={program.startAt + program.duration <= Date.now() || reservationState === 'submitting' || reservationState === 'reserved'}
            on:click={handleReserve}
          >
            {#if program.startAt + program.duration <= Date.now()}
              放送終了
            {:else if reservationState === 'submitting'}
              予約中...
            {:else if reservationState === 'reserved'}
              予約しました
            {:else}
              ● 録画予約
            {/if}
          </button>
          <p class="reservation-help">録画予約は fuba_recorder に登録されます。</p>
          {#if reservationState === 'error'}
            <p class="reservation-error" role="alert">{reservationError}</p>
          {/if}
        </div>
        
        <!-- Additional Info -->
        {#if program.eventId || program.serviceId}
          <div class="additional-info">
            <div class="additional-ids">
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

  .reservation-button {
    width: 100%;
    border-radius: 0.75rem;
    background: #dc2626;
    color: white;
    padding: 0.9rem 1.25rem;
    font-weight: 700;
    transition: background-color 0.2s, opacity 0.2s;
  }

  .reservation-button:hover:not(:disabled) {
    background: #ef4444;
  }

  .reservation-button:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .reservation-button.reserved {
    background: #15803d;
  }

  .reservation-help {
    margin-top: 0.5rem;
    color: #9ca3af;
    font-size: 0.875rem;
    text-align: center;
  }

  .reservation-error {
    margin-top: 0.5rem;
    color: #fca5a5;
    font-size: 0.875rem;
    text-align: center;
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

  /* Minimal monochrome dialog overrides. */
  .modal-backdrop {
    padding: 1.25rem;
    background: rgb(0 0 0 / 80%);
  }

  .modal-content {
    width: min(42rem, 100%);
    height: auto;
    max-height: min(88vh, 52rem);
    border: 1px solid var(--line-strong);
    border-radius: 0;
    background: var(--canvas);
    box-shadow: none;
  }

  .modal-header {
    align-items: flex-start;
    gap: 1rem;
    padding: 1.1rem 1.2rem 1rem;
    border-bottom-color: var(--line);
  }

  .modal-header h2 {
    color: var(--text);
    font-size: clamp(1.05rem, 2.6vw, 1.4rem);
    font-weight: 650;
    line-height: 1.4;
    letter-spacing: -.01em;
  }

  .channel-eyebrow {
    margin-bottom: .3rem;
    color: var(--muted);
    font-size: .72rem;
    font-weight: 600;
    letter-spacing: .06em;
  }

  .close-button {
    width: 2.4rem;
    height: 2.4rem;
    border-radius: .25rem;
    color: var(--muted);
    font-size: 1rem;
  }

  .close-button:hover { background: var(--surface-hover); color: var(--text); }

  .modal-body { padding: 1.15rem 1.2rem 1.3rem; font-size: .95rem; }
  .info-section { display: grid; gap: .45rem; margin-bottom: 1.2rem; }
  .info-row { margin: 0; }
  .info-label { width: 5.5rem; color: var(--muted); font-size: .78rem; font-weight: 600; }
  .info-value { color: var(--text-secondary); font-size: .88rem; }
  .progress-section { padding: .9rem; margin-bottom: 1.2rem; border: 1px solid var(--line); border-radius: 0; background: var(--surface); }
  .progress-meta { display: flex; align-items: center; justify-content: space-between; margin-bottom: .6rem; color: var(--muted); font-size: .78rem; }
  .progress-badge { color: var(--text); font-weight: 650; }
  .progress-track { height: 2px; overflow: hidden; background: var(--surface-active); }
  .progress-value { height: 100%; background: var(--accent); }
  .description-section { margin-bottom: 1.2rem; }
  .description-section h3 { margin-bottom: .5rem; color: var(--muted); font-size: .75rem; font-weight: 600; }
  .description-section p { color: var(--text-secondary); font-size: .9rem; line-height: 1.75; }
  .additional-info { border-top-color: var(--line); }
  .additional-ids { color: var(--muted); font-size: .7rem; }
  .action-section { display: grid; grid-template-columns: 1fr 1fr; gap: .5rem; margin-top: 1.2rem; }
  .watch-button, .reservation-button { min-height: 2.8rem; border-radius: 0; padding: .7rem 1rem; font-size: .85rem; font-weight: 650; }
  .watch-button { display: inline-flex; align-items: center; justify-content: center; gap: .45rem; background: var(--accent); color: #000; }
  .watch-button:hover { background: #e2e2e2; }
  .watch-button svg { width: 1rem; height: 1rem; fill: currentColor; }
  .reservation-button { border: 1px solid var(--line-strong); background: transparent; color: var(--text); }
  .reservation-button:hover:not(:disabled) { background: var(--surface-hover); }
  .reservation-button.reserved { border-color: var(--accent); background: var(--accent-soft); }
  .reservation-help { color: var(--muted); font-size: .72rem; }
  .reservation-error { color: var(--danger); font-size: .72rem; }
  .reservation-help, .reservation-error { grid-column: 1 / -1; margin-top: 0; }

  @media (max-width: 640px) {
    .modal-backdrop { align-items: flex-end; padding: 0; }
    .modal-content { width: 100%; max-height: 92dvh; border-width: 1px 0 0; }
    .modal-header { padding: 1rem .9rem .85rem; }
    .modal-body { padding: .9rem; padding-bottom: calc(.9rem + env(safe-area-inset-bottom)); }
    .action-section { grid-template-columns: 1fr; }
    .watch-button, .reservation-button { min-height: 3rem; }
  }
</style>
