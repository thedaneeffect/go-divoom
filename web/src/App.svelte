<script lang="ts">
  import * as api from './api'

  let status = $state<{ connected: boolean; profile: string } | null>(null)
  let error = $state('')
  let serialPath = $state('')
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

  async function init() {
    try {
      status = await api.getStatus()
      const cfg = await api.getConfig()
      serialPath = cfg.serialPath ?? ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }
  init()

  async function saveConnection() {
    await run(async () => {
      const cfg = await api.getConfig()
      await api.putConfig({ ...cfg, transport: 'serial', serialPath })
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
    <input bind:value={serialPath} placeholder="/dev/cu.Pixoo-Max" />
    <button onclick={saveConnection}>Save</button>
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
  .preview { image-rendering: pixelated; width: 128px; height: 128px; display: block; margin-top: .5rem; }
</style>
