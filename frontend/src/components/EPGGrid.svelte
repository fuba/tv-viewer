<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import type { EPGChannel, EPGProgram, EPGResponse } from '../lib/types/epg'
  import { channelFromEPG } from '../lib/channelSelection'
  import ProgramDetailModal from './ProgramDetailModal.svelte'

  const dispatch = createEventDispatcher()

  // Constants
  const PIXELS_PER_HOUR = 120    // 1 hour = 120px
  const CHANNEL_WIDTH = 140      // Channel column width
  const TIME_AXIS_WIDTH = 50     // Time axis width
  const HEADER_HEIGHT = 40       // Channel header height

  // State
  let channels: EPGChannel[] = []
  let timeRange = { from: 0, to: 0 }
  let currentTime = Date.now()
  let loading = true
  let error: string | null = null
  let activeTab: 'GR' | 'BS' | 'CS' = 'GR'
  let scrollContainer: HTMLElement
  let timeUpdateInterval: ReturnType<typeof setInterval>
  let selectedProgram: EPGProgram | null = null
  let selectedProgramChannel: EPGChannel | null = null
  let isProgramModalOpen = false

  // Computed
  $: filteredChannels = channels.filter(ch => ch.type === activeTab)
  $: totalHeight = timeRange.to > timeRange.from
    ? ((timeRange.to - timeRange.from) / 3600000) * PIXELS_PER_HOUR
    : 0
  $: gridWidth = filteredChannels.length * CHANNEL_WIDTH

  // Time slots for the time axis (every 30 minutes)
  $: timeSlots = generateTimeSlots(timeRange.from, timeRange.to)

  function generateTimeSlots(from: number, to: number): number[] {
    if (from >= to) return []
    const slots: number[] = []
    // Round down to nearest 30 minutes
    let current = Math.floor(from / 1800000) * 1800000
    while (current < to) {
      slots.push(current)
      current += 1800000 // 30 minutes in ms
    }
    return slots
  }

  function formatTime(timestamp: number): string {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })
  }

  function getYPosition(timestamp: number): number {
    return ((timestamp - timeRange.from) / 3600000) * PIXELS_PER_HOUR
  }

  function getProgramHeight(duration: number): number {
    return Math.max((duration / 3600000) * PIXELS_PER_HOUR - 2, 20)
  }

  function getCurrentTimePosition(): number {
    return getYPosition(currentTime)
  }

  function isCurrentProgram(program: EPGProgram): boolean {
    return program.startAt <= currentTime && (program.startAt + program.duration) > currentTime
  }

  async function fetchEPG() {
    loading = true
    error = null
    try {
      const response = await fetch('/api/epg?hours=24')
      if (!response.ok) {
        throw new Error('Failed to fetch EPG data')
      }
      const data: EPGResponse = await response.json()
      channels = data.channels
      timeRange = data.timeRange
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unknown error'
      console.error('EPG fetch error:', e)
    } finally {
      loading = false
    }
  }

  function selectChannel(channel: EPGChannel) {
    dispatch('select', channelFromEPG(channel))
  }

  function watchProgramChannel(event: CustomEvent) {
    dispatch('select', event.detail)
    closeProgram()
  }

  function openProgram(program: EPGProgram, channel: EPGChannel) {
    selectedProgram = program
    selectedProgramChannel = channel
    isProgramModalOpen = true
  }

  function closeProgram() {
    isProgramModalOpen = false
    selectedProgram = null
    selectedProgramChannel = null
  }

  function scrollToCurrentTime() {
    if (scrollContainer && timeRange.from > 0) {
      const currentPosition = getCurrentTimePosition()
      // Scroll to show current time with some context above (show ~30 min before)
      scrollContainer.scrollTop = Math.max(0, currentPosition - PIXELS_PER_HOUR / 2)
    }
  }

  onMount(() => {
    fetchEPG().then(() => {
      // Scroll to current time after data loads
      setTimeout(scrollToCurrentTime, 100)
    })

    // Update current time every minute
    timeUpdateInterval = setInterval(() => {
      currentTime = Date.now()
    }, 60000)
  })

  onDestroy(() => {
    if (timeUpdateInterval) {
      clearInterval(timeUpdateInterval)
    }
  })
</script>

<div class="epg-wrapper">
  <!-- Tab navigation -->
  <div class="epg-tabs">
    <button
      class:active={activeTab === 'GR'}
      on:click={() => activeTab = 'GR'}
    >
      地上波 ({channels.filter(ch => ch.type === 'GR').length})
    </button>
    <button
      class:active={activeTab === 'BS'}
      on:click={() => activeTab = 'BS'}
    >
      BS ({channels.filter(ch => ch.type === 'BS').length})
    </button>
    <button
      class:active={activeTab === 'CS'}
      on:click={() => activeTab = 'CS'}
    >
      CS ({channels.filter(ch => ch.type === 'CS').length})
    </button>

    <button
      class="epg-now"
      on:click={scrollToCurrentTime}
    >
      現在時刻へ
    </button>
  </div>

  {#if loading}
    <p class="epg-message">読み込み中...</p>
  {:else if error}
    <p class="epg-message is-error">{error}</p>
  {:else if filteredChannels.length === 0}
    <p class="epg-message">チャンネルがありません</p>
  {:else}
    <div class="epg-container">
      <!-- Time axis (fixed left) -->
      <div class="time-axis" style="width: {TIME_AXIS_WIDTH}px;">
        <!-- Spacer for header alignment -->
        <div class="time-axis-header" style="height: {HEADER_HEIGHT}px;"></div>

        <div class="time-slots" style="position: relative; height: {totalHeight}px;">
          {#each timeSlots as slot}
            <div
              class="time-slot"
              style="position: absolute; top: {getYPosition(slot)}px; height: 60px;"
            >
              {formatTime(slot)}
            </div>
          {/each}
        </div>
      </div>

      <!-- Channels scroll area -->
      <div class="channels-scroll" bind:this={scrollContainer}>
        <!-- Channel headers (sticky) -->
        <div class="channel-headers" style="width: {gridWidth}px;">
          {#each filteredChannels as channel}
            <button
              class="channel-header"
              style="width: {CHANNEL_WIDTH}px; height: {HEADER_HEIGHT}px;"
              on:click={() => selectChannel(channel)}
              title={channel.name}
            >
              <span class="truncate">{channel.name}</span>
            </button>
          {/each}
        </div>

        <!-- Programs grid -->
        <div class="programs-grid" style="width: {gridWidth}px; height: {totalHeight}px; position: relative;">
          {#each filteredChannels as channel, i}
            <div
              class="channel-column"
              style="position: absolute; left: {i * CHANNEL_WIDTH}px; width: {CHANNEL_WIDTH}px; height: {totalHeight}px;"
            >
              {#each channel.programs as program}
                <button
                  class="program-cell {isCurrentProgram(program) ? 'current' : ''}"
                  style="
                    top: {getYPosition(program.startAt)}px;
                    height: {getProgramHeight(program.duration)}px;
                  "
                  on:click={() => openProgram(program, channel)}
                  title="{formatTime(program.startAt)} - {program.name}"
                >
                  <div class="program-time">{formatTime(program.startAt)}</div>
                  <div class="program-name">{program.name}</div>
                </button>
              {/each}
            </div>
          {/each}

          <!-- Current time indicator -->
          {#if currentTime >= timeRange.from && currentTime <= timeRange.to}
            <div
              class="current-time-line"
              style="top: {getCurrentTimePosition()}px;"
            ></div>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>

<ProgramDetailModal
  program={selectedProgram}
  channel={selectedProgramChannel ? channelFromEPG(selectedProgramChannel) : null}
  isOpen={isProgramModalOpen}
  on:close={closeProgram}
  on:watch={watchProgramChannel}
/>

<style>
  .epg-wrapper {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .epg-tabs { display: flex; align-items: center; gap: .2rem; padding: .35rem max(.5rem, env(safe-area-inset-left)); border-bottom: 1px solid var(--line); }
  .epg-tabs > button:not(.epg-now) { min-height: 2.2rem; padding: 0 .75rem; border-radius: .25rem; color: var(--muted); font-size: .76rem; font-weight: 600; }
  .epg-tabs > button:not(.epg-now):hover { color: var(--text-secondary); }
  .epg-tabs > button.active { background: var(--accent-soft); color: var(--text); }
  .epg-now { min-height: 2.2rem; margin-left: auto; padding: 0 .6rem; border-radius: .25rem; color: var(--text-secondary); font-size: .74rem; }
  .epg-now:hover { background: var(--accent-soft); color: var(--text); }

  .epg-message { flex: 1; display: grid; place-items: center; color: var(--muted); font-size: .85rem; }
  .epg-message.is-error { color: var(--danger); }

  .epg-container {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    background: var(--canvas);
  }

  .time-axis {
    flex-shrink: 0;
    background: var(--surface);
    border-right: 1px solid var(--line);
    overflow: hidden;
  }

  .time-axis-header {
    background: var(--surface-raised);
    border-bottom: 1px solid var(--line);
  }

  .time-slots {
    overflow: hidden;
  }

  .time-slot {
    padding: 4px 8px;
    font-size: 0.7rem;
    color: var(--muted);
    border-bottom: 1px solid var(--line);
    text-align: right;
  }

  .channels-scroll {
    flex: 1;
    overflow: auto;
    position: relative;
  }

  .channel-headers {
    display: flex;
    position: sticky;
    top: 0;
    z-index: 10;
    background: var(--surface-raised);
  }

  .channel-header {
    flex-shrink: 0;
    padding: 8px 4px;
    font-size: 0.7rem;
    font-weight: 500;
    text-align: center;
    border-right: 1px solid var(--line);
    border-bottom: 1px solid var(--line);
    background: var(--surface-raised);
    color: var(--text-secondary);
    cursor: pointer;
    transition: background-color 0.2s;
    overflow: hidden;
  }

  .channel-header:hover {
    background: var(--surface-hover);
    color: var(--text);
  }

  .programs-grid {
    background: var(--canvas);
  }

  .channel-column {
    border-right: 1px solid var(--line);
  }

  .program-cell {
    position: absolute;
    left: 1px;
    right: 1px;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 2px;
    padding: 2px 4px;
    font-size: 0.65rem;
    text-align: left;
    overflow: hidden;
    cursor: pointer;
    transition: background-color 0.2s, border-color 0.2s;
    color: var(--text-secondary);
  }

  .program-cell:hover {
    background: var(--surface-hover);
    border-color: var(--accent);
    z-index: 5;
  }

  .program-cell.current {
    border-left: 2px solid var(--accent);
    background: var(--accent-soft);
    color: var(--text);
  }

  .program-time {
    font-size: 0.6rem;
    color: var(--muted);
    margin-bottom: 1px;
  }

  .program-name {
    line-height: 1.2;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
  }

  .current-time-line {
    position: absolute;
    left: 0;
    right: 0;
    height: 1px;
    background: var(--accent);
    z-index: 20;
    pointer-events: none;
  }

  /* Responsive adjustments */
  @media (max-width: 640px) {
    .epg-tabs > button:not(.epg-now) { flex: 1; padding: 0 .45rem; }
    .epg-now { padding: 0 .35rem; }

    .channel-header {
      font-size: 0.6rem;
      padding: 6px 2px;
    }

    .program-cell {
      font-size: 0.55rem;
      padding: 1px 2px;
    }

    .program-time {
      font-size: 0.5rem;
    }
  }
</style>
