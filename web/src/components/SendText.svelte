<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'

  let text = $state('')

  async function send() {
    const value = text.trim()
    if (!value || device.busy) return
    await device.action(() => api.sendText(value))
  }
</script>

<form
  onsubmit={(e) => {
    e.preventDefault()
    send()
  }}
>
  <label for="text-input">Message</label>
  <input id="text-input" type="text" bind:value={text} placeholder="Scrolls across the display" disabled={device.busy} />
  <button type="submit" class="primary" disabled={device.busy || !text.trim()}>
    {device.busy ? 'Sending…' : 'Send'}
  </button>
</form>

<style>
  form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
</style>
