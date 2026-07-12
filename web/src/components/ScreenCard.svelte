<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'

  async function setScreen(on: boolean) {
    if (device.busy) return
    await device.action(() => api.setScreen(on))
  }
</script>

<div class="screen">
  <button onclick={() => setScreen(true)} disabled={device.busy}>
    {device.busy ? 'Sending…' : 'Turn screen on'}
  </button>
  <button onclick={() => setScreen(false)} disabled={device.busy}>
    {device.busy ? 'Sending…' : 'Turn screen off'}
  </button>
</div>

<style>
  .screen {
    display: flex;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .screen button {
    flex: 1;
  }
</style>
