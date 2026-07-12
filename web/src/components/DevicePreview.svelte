<script lang="ts">
  import { device } from '../lib/device.svelte'
  import { dominantColor } from '../lib/dominantColor'

  let glow = $state('#fe8019')
  let imgEl: HTMLImageElement | undefined = $state()

  // Computed from the decoded image once it loads — for GIFs the <img>'s
  // first painted frame is the first frame, same as the source data.
  function onLoad() {
    if (imgEl) glow = dominantColor(imgEl)
  }
</script>

<figure class="preview" style={`--glow: ${glow}`}>
  <div class="bezel">
    {#if device.lastSent}
      <img
        bind:this={imgEl}
        src={device.lastSent}
        alt="What this browser last sent to the display"
        onload={onLoad}
      />
      <div class="grid" aria-hidden="true"></div>
    {:else}
      <div class="empty">
        <p>Nothing sent yet.</p>
        <p>Drop an image below.</p>
      </div>
    {/if}
  </div>
  <figcaption>Last sent</figcaption>
</figure>

<style>
  .preview {
    margin: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-3);
  }

  .bezel {
    position: relative;
    width: min(72vw, 20rem);
    aspect-ratio: 1;
    background: #000;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-4);
    box-shadow:
      inset 0 2px 10px rgb(0 0 0 / 0.7),
      0 0 3.5rem 0.75rem color-mix(in srgb, var(--glow) 55%, transparent);
    transition: box-shadow 400ms ease;
  }

  .bezel img {
    width: 100%;
    height: 100%;
    display: block;
    image-rendering: pixelated;
    border-radius: calc(var(--radius-lg) / 2);
  }

  .grid {
    position: absolute;
    inset: var(--space-4);
    pointer-events: none;
    border-radius: calc(var(--radius-lg) / 2);
    background-image:
      repeating-linear-gradient(to right, rgb(0 0 0 / 0.3) 0 1px, transparent 1px calc(100% / 32)),
      repeating-linear-gradient(to bottom, rgb(0 0 0 / 0.3) 0 1px, transparent 1px calc(100% / 32));
  }

  .empty {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    gap: var(--space-1);
    color: var(--fg-dim);
  }

  .empty p {
    margin: 0;
    font-size: var(--text-sm);
  }

  figcaption {
    color: var(--fg-dim);
    font-size: var(--text-sm);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
</style>
