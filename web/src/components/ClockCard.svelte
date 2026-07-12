<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'

  let style = $state(0)
  let twentyFour = $state(true)

  async function send() {
    if (device.busy) return
    await device.action(() => api.setClock(style, twentyFour))
  }
</script>

<div class="clock">
  <label for="clock-style">Style (0-15)</label>
  <input id="clock-style" type="number" min="0" max="15" bind:value={style} disabled={device.busy} />
  <label class="checkbox">
    <input type="checkbox" bind:checked={twentyFour} disabled={device.busy} />
    24-hour clock
  </label>
  <button class="primary" onclick={send} disabled={device.busy}>{device.busy ? 'Sending…' : 'Show clock'}</button>
</div>

<style>
  .clock {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .checkbox {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: var(--touch);
    color: var(--fg);
    font-size: var(--text-base);
  }
</style>
