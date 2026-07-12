<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'

  let color = $state('#fe8019')

  // colorSwatchURL renders a flat 32x32 fill so the preview panel can show
  // "what was sent" for a light command the same way it shows a real
  // image — a solid data URL, not a photo of the device.
  function colorSwatchURL(hex: string): string | null {
    const canvas = document.createElement('canvas')
    canvas.width = 32
    canvas.height = 32
    const ctx = canvas.getContext('2d')
    if (!ctx) return null
    ctx.fillStyle = hex
    ctx.fillRect(0, 0, 32, 32)
    return canvas.toDataURL('image/png')
  }

  async function send() {
    if (device.busy) return
    const ok = await device.action(() => api.setLight(color))
    if (ok) {
      const url = colorSwatchURL(color)
      if (url) device.setLastSent(url)
    }
  }
</script>

<div class="light">
  <label for="light-color">Color</label>
  <div class="row">
    <input id="light-color" type="color" bind:value={color} disabled={device.busy} />
    <span class="mono">{color}</span>
  </div>
  <button onclick={send} disabled={device.busy}>{device.busy ? 'Sending…' : 'Show color'}</button>
</div>

<style>
  .light {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
</style>
