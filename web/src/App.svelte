<script lang="ts">
  import { device } from './lib/device.svelte'
  import StatusChip from './components/StatusChip.svelte'
  import ConnectionPanel from './components/ConnectionPanel.svelte'
  import DevicePreview from './components/DevicePreview.svelte'
  import BrightnessSlider from './components/BrightnessSlider.svelte'
  import Card from './components/Card.svelte'
  import SendImage from './components/SendImage.svelte'
  import SendText from './components/SendText.svelte'
  import LightCard from './components/LightCard.svelte'
  import ClockCard from './components/ClockCard.svelte'
  import ScreenCard from './components/ScreenCard.svelte'

  let connectionExpanded = $state(false)

  $effect(() => {
    device.refresh()
    device.startPolling()
    return () => device.stopPolling()
  })

  // Auto-expand the connection panel whenever the device isn't connected,
  // so the fix is never more than a glance away. Once a user has expanded
  // or collapsed it manually, leave that alone until connectivity changes
  // again.
  $effect(() => {
    if (device.status && !device.status.connected) connectionExpanded = true
  })

  function toggleConnection() {
    connectionExpanded = !connectionExpanded
  }
</script>

<main>
  <header>
    <h1>go-divoom</h1>
    <StatusChip expanded={connectionExpanded} ontoggle={toggleConnection} />
  </header>

  <div class="banner-region" aria-live="polite">
    {#if device.error}
      <div class="banner">
        <p>{device.error}</p>
        <button onclick={() => device.dismissError()} aria-label="Dismiss error">×</button>
      </div>
    {/if}
  </div>

  <ConnectionPanel expanded={connectionExpanded} />

  <section class="hero">
    <DevicePreview />
    <BrightnessSlider />
  </section>

  <div class="grid">
    <Card title="Image / GIF"><SendImage /></Card>
    <Card title="Text"><SendText /></Card>
    <Card title="Light"><LightCard /></Card>
    <Card title="Clock"><ClockCard /></Card>
    <Card title="Screen"><ScreenCard /></Card>
  </div>
</main>

<style>
  main {
    max-width: 40rem;
    margin: 0 auto;
    padding: var(--space-6) var(--space-4) var(--space-8);
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  h1 {
    font-size: var(--text-xl);
    font-weight: 600;
    margin: 0;
  }

  .hero {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-4) 0;
  }

  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--space-4);
  }

  @media (min-width: 640px) {
    .grid {
      grid-template-columns: 1fr 1fr;
    }
  }

  .banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    background: color-mix(in srgb, var(--err) 18%, var(--surface));
    border: 1px solid var(--err);
    border-radius: var(--radius);
    padding: var(--space-3) var(--space-4);
  }

  .banner p {
    margin: 0;
  }

  .banner button {
    background: none;
    border: none;
    min-height: var(--touch);
    min-width: var(--touch);
    color: var(--fg);
    font-size: var(--text-lg);
    padding: 0;
  }
</style>
