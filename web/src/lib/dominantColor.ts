// dominantColor estimates the most visually prominent color in an already
// decoded <img>, used to tint the preview panel's bloom. It downsamples to
// a small canvas and buckets pixels by coarse RGB, weighting each pixel by
// how saturated and mid-toned it is — so a vivid subject wins over a large
// flat white/black/gray background instead of just picking the most common
// raw color.
//
// Any failure (a tainted canvas, a decode error, an environment with no
// canvas support) falls back to the Gruvbox accent color rather than
// throwing, since this only drives a decorative glow — never the thing
// actually being sent to the device.
const FALLBACK = '#fe8019'
const SAMPLE_SIZE = 32

export function dominantColor(img: HTMLImageElement): string {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = SAMPLE_SIZE
    canvas.height = SAMPLE_SIZE
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) return FALLBACK

    ctx.drawImage(img, 0, 0, SAMPLE_SIZE, SAMPLE_SIZE)
    const { data } = ctx.getImageData(0, 0, SAMPLE_SIZE, SAMPLE_SIZE)

    const buckets = new Map<string, { r: number; g: number; b: number; weight: number }>()

    for (let i = 0; i < data.length; i += 4) {
      const r = data[i]
      const g = data[i + 1]
      const b = data[i + 2]
      const a = data[i + 3]
      if (a < 16) continue // skip near-transparent pixels

      const max = Math.max(r, g, b)
      const min = Math.min(r, g, b)
      const lightness = (max + min) / 2 / 255
      const saturation = max === min ? 0 : (max - min) / (255 - Math.abs(max + min - 255))
      // Full weight at mid-lightness, tapering to 0 at black/white, plus a
      // small floor so fully desaturated pixels still count for something.
      const weight = saturation * (1 - Math.abs(lightness - 0.5) * 2) + 0.01

      const key = `${r >> 4}-${g >> 4}-${b >> 4}`
      const bucket = buckets.get(key) ?? { r: 0, g: 0, b: 0, weight: 0 }
      bucket.r += r * weight
      bucket.g += g * weight
      bucket.b += b * weight
      bucket.weight += weight
      buckets.set(key, bucket)
    }

    let best: { r: number; g: number; b: number; weight: number } | undefined
    for (const bucket of buckets.values()) {
      if (!best || bucket.weight > best.weight) best = bucket
    }
    if (!best || best.weight <= 0) return FALLBACK

    const r = Math.round(best.r / best.weight)
    const g = Math.round(best.g / best.weight)
    const b = Math.round(best.b / best.weight)
    return `#${[r, g, b].map((v) => v.toString(16).padStart(2, '0')).join('')}`
  } catch {
    return FALLBACK
  }
}
