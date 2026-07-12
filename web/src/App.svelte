<script lang="ts">
  import * as api from './api'
  import type { DeviceInfo } from './api'

  type ConnectionConfig = { transport: string; serialPath: string; mac: string; [k: string]: unknown }

  let status = $state<{ connected: boolean; profile: string } | null>(null)
  let error = $state('')
  let cfg = $state<ConnectionConfig | null>(null)
  let manualTarget = $state('')
  let scanning = $state(false)
  let devices = $state<DeviceInfo[]>([])
  let scanNote = $state('')
  let brightness = $state(100)
  let text = $state('')
  let lightColor = $state('#ff8800')
  let clockStyle = $state(0)
  let preview = $state('')

  async function run(fn: () => Promise<unknown>) {
    error = ''
    try {
      await fn()
      status = await api.getStatus()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function refreshConnection() {
    status = await api.getStatus()
    cfg = await api.getConfig()
  }

  async function init() {
    try {
      await refreshConnection()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }
  init()

  // scanForDevices runs the same Bluetooth inquiry `divoom devices` does.
  // It takes several seconds, hence the disabled/spinner state — and a
  // device that's currently connected won't answer, so it can be absent
  // from the results even though it's paired and nearby.
  async function scanForDevices() {
    error = ''
    scanning = true
    devices = []
    scanNote = ''
    try {
      const res = await api.getDevices()
      devices = res.devices ?? []
      scanNote = res.note ?? ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      scanning = false
    }
  }

  async function useDevice(mac: string) {
    await run(async () => {
      const current = await api.getConfig()
      await api.putConfig({ ...current, transport: 'rfcomm', mac })
      await refreshConnection()
    })
  }

  // saveManual mirrors `divoom use <mac|serial-path>`: it never hardcodes
  // a transport, it infers one from the shape of the input.
  async function saveManual() {
    const target = manualTarget.trim()
    if (!target) return
    await run(async () => {
      const current = await api.getConfig()
      const patch = target.includes('/')
        ? { transport: 'serial', serialPath: target }
        : { transport: 'rfcomm', mac: target }
      await api.putConfig({ ...current, ...patch })
      await refreshConnection()
    })
  }

  function onFile(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0]
    if (!file) return
    preview = URL.createObjectURL(file)
    run(() => api.sendImage(file))
  }
</script>

<main>
  <h1>go-divoom</h1>
  <p class="status">
    {status ? `${status.profile} — ${status.connected ? 'connected' : 'not connected'}` : '…'}
  </p>
  {#if error}<p class="error">{error}</p>{/if}

  <section>
    <h2>Connection</h2>
    <p class="status">
      {#if cfg}
        {cfg.transport === 'rfcomm'
          ? `Bluetooth (rfcomm) — ${cfg.mac || 'no device set'}`
          : `Serial — ${cfg.serialPath || 'no path set'}`}
        {status ? (status.connected ? ' — connected' : ' — not connected') : ''}
      {:else}
        …
      {/if}
    </p>
    {#if cfg && cfg.transport !== 'rfcomm'}
      <p class="note">Bluetooth (rfcomm) is recommended on macOS — serial uses the unreliable /dev/cu.* path.</p>
    {/if}

    <button onclick={scanForDevices} disabled={scanning}>
      {scanning ? 'Scanning… (few seconds)' : 'Scan for devices'}
    </button>
    <p class="hint">A device that's currently connected won't answer a scan — disconnect it first if it's missing.</p>
    {#if scanNote}<p class="note">{scanNote}</p>{/if}
    {#if devices.length}
      <ul class="devices">
        {#each devices as d}
          <li><button onclick={() => useDevice(d.mac)}>{d.name} — {d.mac}</button></li>
        {/each}
      </ul>
    {/if}

    <div class="manual">
      <input bind:value={manualTarget} placeholder="AA:BB:CC:DD:EE:FF or /dev/cu.Pixoo-Max" />
      <button onclick={saveManual}>Save</button>
    </div>
  </section>

  <section>
    <h2>Image / GIF</h2>
    <input type="file" accept="image/png,image/jpeg,image/gif" onchange={onFile} />
    {#if preview}<img class="preview" src={preview} alt="preview" />{/if}
  </section>

  <section>
    <h2>Text</h2>
    <input bind:value={text} placeholder="Message" />
    <button onclick={() => run(() => api.sendText(text))}>Send</button>
  </section>

  <section>
    <h2>Controls</h2>
    <label>
      Brightness {brightness}
      <input
        type="range" min="0" max="100" bind:value={brightness}
        onchange={() => run(() => api.setBrightness(brightness))}
      />
    </label>
    <div>
      <button onclick={() => run(() => api.setScreen(true))}>Screen on</button>
      <button onclick={() => run(() => api.setScreen(false))}>Screen off</button>
    </div>
    <div>
      <input type="color" bind:value={lightColor} />
      <button onclick={() => run(() => api.setLight(lightColor, brightness))}>Light</button>
    </div>
    <div>
      <label>
        Clock style
        <input type="number" min="0" max="15" bind:value={clockStyle} />
      </label>
      <button onclick={() => run(() => api.setClock(clockStyle, true))}>Show clock</button>
    </div>
  </section>
</main>

<style>
  main { max-width: 28rem; margin: 2rem auto; font-family: system-ui, sans-serif; }
  section { margin: 1.5rem 0; }
  .status { color: #666; }
  .error { color: #c00; }
  .note, .hint { color: #888; font-size: .875rem; margin: .35rem 0; }
  .devices { list-style: none; padding: 0; margin: .5rem 0; }
  .devices li { margin: .25rem 0; }
  .manual { margin-top: .75rem; }
  .preview { image-rendering: pixelated; width: 128px; height: 128px; display: block; margin-top: .5rem; }
</style>
