<script lang="ts">
  export let isOpen = false
  export let title = ''
  export let fullWidth = false
  let backdropPressed = false

  function handleBackdropPointerDown(e: PointerEvent) {
    backdropPressed = e.target === e.currentTarget
  }

  function handleBackdropClick(e: MouseEvent) {
    if (backdropPressed && e.target === e.currentTarget) {
      isOpen = false
    }
    backdropPressed = false
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      isOpen = false
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if isOpen}
  <!-- Background overlay -->
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="panel-backdrop"
    on:pointerdown={handleBackdropPointerDown}
    on:click={handleBackdropClick}
  >
    <!-- Panel body -->
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div
      class="overlay-panel {fullWidth ? 'overlay-wide' : ''}"
      on:click|stopPropagation
    >
      <!-- Header -->
      <div class="overlay-header">
        <h2>{title}</h2>
        <button
          class="overlay-close"
          on:click={() => isOpen = false}
          aria-label="閉じる"
        >
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <!-- Content -->
      <div class="overlay-content">
        <slot />
      </div>
    </div>
  </div>
{/if}
