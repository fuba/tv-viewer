<script lang="ts">
  import type { SubtitleMessage } from '../lib/webrtc/types'
  import { captionBands, captionSpanStyle, hasPlacedLayout } from '../lib/captionLayout'

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
      <!-- The background is painted first, as one band per stretch of caption. -->
      {#each caption.rows ?? [] as row}
        {#each captionBands(row, plane) as band}
          <span
            class="caption-band"
            style="
              left: {band.left}%;
              bottom: {band.bottom}%;
              width: {band.width}%;
              height: {band.height}%;
              background: {band.background};
              opacity: {band.opacity};
            "
          ></span>
        {/each}
      {/each}
      {#each caption.rows ?? [] as row}
        {#each row.spans ?? [] as span}
          {@const style = captionSpanStyle(span, row, plane)}
          <span
            class="caption-span"
            style="
              left: {style.left}%;
              bottom: {style.bottom}%;
              width: {style.width}%;
              height: {style.height}%;
              font-size: {style.fontSize}cqh;
              color: {style.color};
              opacity: {style.opacity};
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

  .caption-band {
    position: absolute;
    display: block;
  }

  .caption-span {
    position: absolute;
    display: flex;
    align-items: center;
    box-sizing: border-box;
    line-height: 1;
    white-space: pre;
  }

  .caption-run {
    position: relative;
    display: inline-block;
    transform-origin: left center;
  }
</style>
