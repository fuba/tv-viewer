<script lang="ts">
  import type { SubtitleMessage } from '../lib/webrtc/types'
  import { captionSpanStyle, hasPlacedLayout } from '../lib/captionLayout'

  // ARIB draws captions on a fixed plane (960x540 for HD) and places every run of
  // characters at an absolute position on it. Drawing them the same way keeps the
  // broadcaster's line breaks, ruby (a small run sitting above its base text) and
  // placement, none of which survive in the flattened text.
  export let captions: SubtitleMessage[] = []

  $: placed = captions.filter(hasPlacedLayout)
  $: plainText = captions
    .filter(caption => !hasPlacedLayout(caption))
    .map(caption => caption.text ?? '')
    .filter(Boolean)
    .join('\n')

</script>

{#if placed.length}
  <div class="caption-plane" aria-live="polite">
    {#each placed as caption}
      {@const plane = caption.plane ?? { width: 0, height: 0 }}
      {#each caption.rows ?? [] as row}
        {#each row.spans ?? [] as span}
          {@const style = captionSpanStyle(span, row, plane)}
          <span
            class="caption-span"
            style="
              left: {style.left}%;
              bottom: {style.bottom}%;
              min-width: {style.minWidth}%;
              height: {style.height}%;
              font-size: {style.fontSize}cqh;
              color: {style.color};
              opacity: {style.opacity};
              --caption-background: {style.background};
              --caption-background-opacity: {style.backgroundOpacity};
            "
          >
            <span
              class="caption-run"
              style="letter-spacing: {style.letterSpacing}em; transform: scaleX({style.scaleX});"
            >{span.text}</span>
          </span>
        {/each}
      {/each}
    {/each}
  </div>
{/if}

{#if plainText}
  <div class="live-subtitle-layer" aria-live="polite">
    <div class="live-subtitle-body">{plainText}</div>
  </div>
{/if}

<style>
  .caption-plane {
    position: absolute;
    inset: 0;
    z-index: 10;
    pointer-events: none;
    font-family: 'WLMaru2004Emoji', 'Hiragino Kaku Gothic ProN', 'Hiragino Sans', Meiryo, sans-serif;
  }

  .caption-span {
    position: absolute;
    display: flex;
    align-items: center;
    box-sizing: border-box;
    line-height: 1;
    white-space: pre;
  }

  /* The drawn caption box sits behind the glyphs and keeps its own opacity. */
  .caption-span::before {
    content: '';
    position: absolute;
    inset: 0;
    background: var(--caption-background, #000);
    opacity: var(--caption-background-opacity, 1);
  }

  .caption-run {
    position: relative;
    display: inline-block;
    transform-origin: left center;
  }
</style>
