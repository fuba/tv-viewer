<script lang="ts">
  export let isOpen = false
  export let title = ''
  export let fullWidth = false

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      isOpen = false
    }
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
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70"
    on:click={handleBackdropClick}
  >
    <!-- Panel body -->
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div
      class="bg-gray-800 rounded-lg max-h-[85vh] flex flex-col
             {fullWidth ? 'w-[95vw] max-w-6xl' : 'w-[90vw] max-w-2xl'}"
      on:click|stopPropagation
    >
      <!-- Header -->
      <div class="flex items-center justify-between p-4 border-b border-gray-700">
        <h2 class="text-lg font-semibold text-white">{title}</h2>
        <button
          class="p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white"
          on:click={() => isOpen = false}
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <!-- Content -->
      <div class="p-4 overflow-y-auto flex-1">
        <slot />
      </div>
    </div>
  </div>
{/if}
