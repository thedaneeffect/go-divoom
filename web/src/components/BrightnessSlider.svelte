<script lang="ts">
  import { device } from '../lib/device.svelte'
  import * as api from '../api'

  const DEBOUNCE_MS = 150

  // The device has no readback for its current brightness, so 100 (the
  // device's own power-on default) is just this panel's starting guess,
  // not a synced value.
  let value = $state(100)
  let timer: ReturnType<typeof setTimeout> | undefined

  function onInput() {
    clearTimeout(timer)
    timer = setTimeout(commit, DEBOUNCE_MS)
  }

  function commit() {
    device.action(() => api.setBrightness(value))
  }

  $effect(() => () => clearTimeout(timer))
</script>

<div class="brightness">
  <label for="brightness-input">
    Brightness <span class="mono value">{value}</span>
  </label>
  <input
    id="brightness-input"
    type="range"
    min="0"
    max="100"
    step="1"
    bind:value
    oninput={onInput}
    aria-valuetext={`${value} percent`}
  />
</div>

<style>
  .brightness {
    width: min(72vw, 20rem);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  label {
    display: flex;
    justify-content: space-between;
  }

  input[type='range'] {
    width: 100%;
  }
</style>
