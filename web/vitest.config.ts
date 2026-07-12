import { defineConfig } from 'vitest/config'

// Only lib/target.ts's pure helpers are unit-tested (see its file header) —
// no component-rendering harness, so a plain Node environment is enough.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
