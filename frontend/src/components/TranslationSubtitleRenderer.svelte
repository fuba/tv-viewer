<script lang="ts">
  import type { TranslationCaptionState } from '../lib/translationCaption'

  export let caption: TranslationCaptionState | null = null
</script>

{#if caption}
  <div class="translation-subtitle-layer" aria-live={caption.phase === 'final' ? 'polite' : 'off'}>
    <div class="translation-subtitle-body">
      <span class="translation-credit">遅延翻訳 · VOICEVOX:ずんだもん</span>
      <span class="translation-text">{caption.text}</span>
    </div>
  </div>
{/if}
