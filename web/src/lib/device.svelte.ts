// device.svelte.ts is the single source of truth for the panel's view of
// the device: connection status, persisted config, the last error, and
// whether any request is in flight. Components read it directly (no prop
// drilling) and route mutations through `action`, which is the one place
// busy-tracking, error handling, and post-action status refresh happen.
import * as api from '../api'
import { errorMessage } from './target'

const POLL_INTERVAL_MS = 5000

class DeviceStore {
  status = $state<api.Status | null>(null)
  config = $state<api.DeviceConfig | null>(null)
  error = $state('')
  // Object/data URL for whatever this browser last successfully sent as a
  // 32x32 image (a real image send, or a solid-color swatch standing in
  // for a light command). Never a live mirror of the device — the device
  // has no framebuffer readback.
  lastSent = $state<string | null>(null)

  #inFlight = $state(0)
  #pollHandle: ReturnType<typeof setInterval> | undefined

  get busy(): boolean {
    return this.#inFlight > 0
  }

  // refresh reloads status + config from the server. Failures surface
  // through `error` like any other action.
  async refresh(): Promise<void> {
    try {
      const [status, config] = await Promise.all([api.getStatus(), api.getConfig()])
      this.status = status
      this.config = config
    } catch (e) {
      this.error = errorMessage(e)
    }
  }

  // action runs fn as a tracked unit of work: busy is up for its duration
  // (so UI can disable controls / show an in-flight label), failures
  // surface through `error`, and status is refreshed afterwards so the
  // panel reflects whatever the device just did. Overlapping actions are
  // allowed (busy is a count, not a lock) — e.g. a brightness debounce can
  // settle while a text send is still in flight — since the server already
  // serializes actual device access; busy here is strictly a UI signal.
  async action(fn: () => Promise<unknown>): Promise<boolean> {
    this.#inFlight++
    this.error = ''
    try {
      await fn()
      await this.refresh()
      return true
    } catch (e) {
      this.error = errorMessage(e)
      return false
    } finally {
      this.#inFlight--
    }
  }

  // setLastSent records a new "last sent" preview image, releasing the
  // previous object URL (if any) so successive sends don't leak blob URLs
  // over a long-lived session.
  setLastSent(url: string): void {
    if (this.lastSent) URL.revokeObjectURL(this.lastSent)
    this.lastSent = url
  }

  dismissError(): void {
    this.error = ''
  }

  // startPolling begins a 5s status refresh loop. A tick that lands while
  // an action is in flight is skipped rather than run concurrently with
  // it — the action's own post-completion refresh will catch the panel up
  // a moment later, so nothing is lost, and the poll can't race a mutation
  // that's still resolving.
  startPolling(): void {
    if (this.#pollHandle) return
    this.#pollHandle = setInterval(() => {
      if (!this.busy) this.refresh()
    }, POLL_INTERVAL_MS)
  }

  stopPolling(): void {
    clearInterval(this.#pollHandle)
    this.#pollHandle = undefined
  }
}

export const device = new DeviceStore()
