// target.ts holds pure, DOM-free helpers shared by the connection UI. Kept
// separate from any component so they can be unit-tested directly — this is
// the logic that caused a real bug (a hardcoded transport silently
// overwriting a working rfcomm config with an empty serial one).

export type Target = { transport: 'serial'; serialPath: string } | { transport: 'rfcomm'; mac: string }

// inferTarget mirrors `divoom use <mac|serial-path>` (cmd/divoom/use.go's
// cmdUse): anything containing "/" is a serial device path, everything
// else is assumed to be a Bluetooth MAC address. It must stay in sync with
// that detection — this is the single place the panel decides which
// transport a manually-entered or scanned target implies.
export function inferTarget(input: string): Target {
  const target = input.trim()
  if (target.includes('/')) {
    return { transport: 'serial', serialPath: target }
  }
  return { transport: 'rfcomm', mac: target }
}

// errorMessage extracts a human-readable message from anything a catch
// block might hand back, since thrown values in JS are not guaranteed to
// be Error instances.
export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  return String(e)
}
