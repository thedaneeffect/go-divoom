async function req<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error ?? `HTTP ${res.status}`)
  return body as T
}

const post = <T = void>(path: string, data: unknown): Promise<T> =>
  req<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })

export type DeviceInfo = { name: string; mac: string }
export type Status = { connected: boolean; profile: string; transport: string }
export type DeviceConfig = {
  transport: string
  serialPath: string
  mac: string
  channel: number
  listenAddr: string
}

export const getStatus = (): Promise<Status> => req('/api/status')
export const getConfig = (): Promise<DeviceConfig> => req('/api/config')
export const putConfig = (cfg: DeviceConfig): Promise<void> =>
  req('/api/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
// getDevices scans for nearby Bluetooth devices (same scan as `divoom
// devices`). It takes several seconds — that's an inquiry scan, not a
// bug — so callers should show a busy state while it's in flight. A
// missing scanner on this platform comes back as an empty list plus a
// `note` explaining how to find the MAC by hand, not an error.
export const getDevices = (): Promise<{ devices: DeviceInfo[]; note?: string }> => req('/api/devices')
export const setBrightness = (value: number): Promise<void> => post('/api/brightness', { value })
export const setScreen = (on: boolean): Promise<void> => post('/api/screen', { on })
export const setLight = (color: string, brightness?: number): Promise<void> =>
  post('/api/light', brightness === undefined ? { color } : { color, brightness })
export const setClock = (style: number, twentyFour: boolean): Promise<void> =>
  post('/api/clock', { style, twentyFour })
export const sendText = (text: string): Promise<void> => post('/api/text', { text })

export async function sendImage(file: File): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch('/api/image', { method: 'POST', body: form })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? `HTTP ${res.status}`)
  }
}
