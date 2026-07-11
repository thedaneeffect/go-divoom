async function req(path: string, init?: RequestInit): Promise<any> {
  const res = await fetch(path, init)
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error ?? `HTTP ${res.status}`)
  return body
}

const post = (path: string, data: unknown) =>
  req(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })

export const getStatus = () => req('/api/status')
export const getConfig = () => req('/api/config')
export const putConfig = (cfg: unknown) =>
  req('/api/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
export const setBrightness = (value: number) => post('/api/brightness', { value })
export const setScreen = (on: boolean) => post('/api/screen', { on })
export const setLight = (color: string, brightness: number) =>
  post('/api/light', { color, brightness })
export const setClock = (style: number, twentyFour: boolean) =>
  post('/api/clock', { style, twentyFour })
export const sendText = (text: string) => post('/api/text', { text })

export async function sendImage(file: File): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch('/api/image', { method: 'POST', body: form })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? `HTTP ${res.status}`)
  }
}
