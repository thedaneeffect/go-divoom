<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'

  const ACCEPTED = ['image/png', 'image/jpeg', 'image/gif']

  let fileName = $state('')
  let localError = $state('')
  let dragOver = $state(false)

  function accept(file: File): boolean {
    if (!ACCEPTED.includes(file.type)) {
      localError = `"${file.name}" isn't a supported image type. Use PNG, JPEG, or GIF.`
      return false
    }
    localError = ''
    return true
  }

  async function send(file: File) {
    if (device.busy) return
    if (!accept(file)) return
    fileName = file.name
    const url = URL.createObjectURL(file)
    const ok = await device.action(() => api.sendImage(file))
    if (ok) {
      device.setLastSent(url)
    } else {
      URL.revokeObjectURL(url)
    }
  }

  function onFileInput(e: Event) {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    // Reset so re-picking the same file (e.g. retrying a failed send)
    // reliably fires change again.
    input.value = ''
    if (file) send(file)
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    dragOver = false
    const file = e.dataTransfer?.files?.[0]
    if (file) send(file)
  }

  function onDragOver(e: DragEvent) {
    e.preventDefault()
    dragOver = true
  }

  function onDragLeave() {
    dragOver = false
  }
</script>

<label
  class="dropzone"
  class:dragover={dragOver}
  ondrop={onDrop}
  ondragover={onDragOver}
  ondragleave={onDragLeave}
>
  <input
    type="file"
    accept="image/png,image/jpeg,image/gif"
    onchange={onFileInput}
    disabled={device.busy}
    aria-describedby={localError ? 'image-error' : undefined}
  />
  <span>{device.busy ? 'Sending…' : 'Drop an image or GIF here, or click to browse.'}</span>
  {#if fileName}<span class="filename mono">{fileName}</span>{/if}
</label>
{#if localError}<p id="image-error" class="field-error">{localError}</p>{/if}

<style>
  .dropzone {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    min-height: 7rem;
    text-align: center;
    padding: var(--space-4);
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    background: var(--surface-2);
    cursor: pointer;
    font-size: var(--text-sm);
    color: var(--fg-dim);
  }

  .dropzone.dragover {
    border-color: var(--accent);
    color: var(--fg);
  }

  .dropzone:has(input:disabled) {
    opacity: 0.6;
    cursor: default;
  }

  .dropzone input[type='file'] {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    opacity: 0;
    min-height: 0;
  }

  .filename {
    color: var(--fg);
  }
</style>
