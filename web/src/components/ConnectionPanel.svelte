<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'
  import type { DeviceInfo } from '../api'
  import { errorMessage, inferTarget, type Target } from '../lib/target'

  let { expanded }: { expanded: boolean } = $props()

  let manualTarget = $state('')
  let saving = $state(false)
  let saveError = $state('')

  let scanning = $state(false)
  let devices = $state<DeviceInfo[]>([])
  let scanNote = $state('')
  let scanError = $state('')

  // applyTarget re-fetches the current config immediately before merging
  // in the patch (rather than reusing a possibly-stale in-memory copy) so
  // a manual save or "use this device" click can't clobber fields changed
  // elsewhere since the panel last loaded.
  async function applyTarget(target: Target) {
    saving = true
    saveError = ''
    try {
      const current = await api.getConfig()
      await api.putConfig({ ...current, ...target })
      await device.refresh()
    } catch (e) {
      saveError = errorMessage(e)
    } finally {
      saving = false
    }
  }

  function useDevice(mac: string) {
    applyTarget({ transport: 'rfcomm', mac })
  }

  function saveManual() {
    const value = manualTarget.trim()
    if (!value) return
    applyTarget(inferTarget(value))
  }

  // scan runs the same Bluetooth inquiry `divoom devices` does. It takes
  // several seconds, hence the disabled/label-change state — and a device
  // that's currently connected won't answer, so it can be absent from the
  // results even though it's paired and nearby.
  async function scan() {
    scanning = true
    scanError = ''
    devices = []
    scanNote = ''
    try {
      const res = await api.getDevices()
      devices = res.devices ?? []
      scanNote = res.note ?? ''
    } catch (e) {
      scanError = errorMessage(e)
    } finally {
      scanning = false
    }
  }

  let transportLabel = $derived(device.config ? (device.config.transport === 'rfcomm' ? 'Bluetooth' : 'Serial') : '')
  let targetValue = $derived(
    device.config
      ? device.config.transport === 'rfcomm'
        ? device.config.mac || 'no device set'
        : device.config.serialPath || 'no path set'
      : '',
  )
</script>

<div id="connection-panel" class="panel" hidden={!expanded}>
  <p>
    Current:
    {#if device.config}
      {transportLabel} — <span class="mono">{targetValue}</span>
    {:else}
      Loading…
    {/if}
    {#if device.status}— {device.status.connected ? 'connected' : 'not connected'}{/if}
  </p>
  {#if device.config && device.config.transport !== 'rfcomm'}
    <p class="hint">Bluetooth is recommended on macOS — serial uses the unreliable /dev/cu.* path.</p>
  {/if}

  <div class="scan">
    <button onclick={scan} disabled={scanning}>
      {scanning ? 'Scanning… (few seconds)' : 'Scan for devices'}
    </button>
    <p class="hint">A device that's currently connected won't answer a scan — disconnect it first if it's missing.</p>
    {#if scanError}<p class="field-error">{scanError}</p>{/if}
    {#if scanNote}<p class="hint">{scanNote}</p>{/if}
    {#if devices.length}
      <ul class="devices">
        {#each devices as d (d.mac)}
          <li>
            <button onclick={() => useDevice(d.mac)} disabled={saving}>
              <span class="mono">{d.name} — {d.mac}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <form
    class="manual"
    onsubmit={(e) => {
      e.preventDefault()
      saveManual()
    }}
  >
    <label for="manual-target">MAC address or serial path</label>
    <input
      id="manual-target"
      type="text"
      bind:value={manualTarget}
      placeholder="AA:BB:CC:DD:EE:FF or /dev/cu.Pixoo-Max"
      class="mono"
      disabled={saving}
    />
    <button type="submit" class="primary" disabled={saving || !manualTarget.trim()}>
      {saving ? 'Saving…' : 'Save'}
    </button>
  </form>
  {#if saveError}<p class="field-error">{saveError}</p>{/if}
</div>

<style>
  .panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-4);
  }

  .panel[hidden] {
    display: none;
  }

  .scan,
  .manual {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    align-items: flex-start;
  }

  .devices {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: 100%;
  }

  .devices button {
    width: 100%;
    text-align: left;
  }

  .manual {
    width: 100%;
  }

  .manual input {
    width: 100%;
  }
</style>
