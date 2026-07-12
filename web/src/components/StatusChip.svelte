<script lang="ts">
  import { device } from '../lib/device.svelte'

  let { expanded, ontoggle }: { expanded: boolean; ontoggle: () => void } = $props()

  let state = $derived(
    device.status === null ? 'connecting' : device.status.connected ? 'connected' : 'disconnected',
  )
  let label = $derived(
    state === 'connecting' ? 'Connecting' : state === 'connected' ? 'Connected' : 'Not connected',
  )
</script>

<button
  type="button"
  class="chip"
  class:connected={state === 'connected'}
  class:disconnected={state === 'disconnected'}
  aria-expanded={expanded}
  aria-controls="connection-panel"
  onclick={ontoggle}
>
  <span class="dot" aria-hidden="true"></span>
  {label}
</button>

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    background: var(--surface);
    border-radius: 999px;
    font-size: var(--text-sm);
  }

  .dot {
    width: 0.625rem;
    height: 0.625rem;
    border-radius: 50%;
    background: var(--fg-dim);
    flex-shrink: 0;
  }

  .chip.connected .dot {
    background: var(--ok);
  }

  .chip.disconnected .dot {
    background: var(--err);
  }
</style>
