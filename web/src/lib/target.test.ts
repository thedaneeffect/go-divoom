import { describe, expect, it } from 'vitest'
import { errorMessage, inferTarget } from './target'

describe('inferTarget', () => {
  it('treats a full serial device path as serial', () => {
    expect(inferTarget('/dev/cu.Pixoo-Max')).toEqual({
      transport: 'serial',
      serialPath: '/dev/cu.Pixoo-Max',
    })
  })

  it('treats a short /dev path as serial (edge case: still just needs a slash)', () => {
    expect(inferTarget('/dev/cu.Foo')).toEqual({
      transport: 'serial',
      serialPath: '/dev/cu.Foo',
    })
  })

  it('treats a bare MAC address as rfcomm', () => {
    expect(inferTarget('AA:BB:CC:DD:EE:FF')).toEqual({
      transport: 'rfcomm',
      mac: 'AA:BB:CC:DD:EE:FF',
    })
  })

  it('treats a lowercase MAC address as rfcomm', () => {
    expect(inferTarget('aa:bb:cc:dd:ee:ff')).toEqual({
      transport: 'rfcomm',
      mac: 'aa:bb:cc:dd:ee:ff',
    })
  })

  it('treats any relative path containing a slash as serial', () => {
    expect(inferTarget('relative/path')).toEqual({
      transport: 'serial',
      serialPath: 'relative/path',
    })
  })

  it('trims surrounding whitespace before inferring and before storing', () => {
    expect(inferTarget('  AA:BB:CC:DD:EE:FF  ')).toEqual({
      transport: 'rfcomm',
      mac: 'AA:BB:CC:DD:EE:FF',
    })
    expect(inferTarget('  /dev/cu.Foo  ')).toEqual({
      transport: 'serial',
      serialPath: '/dev/cu.Foo',
    })
  })

  it('falls back to rfcomm for an empty string (no slash present)', () => {
    expect(inferTarget('')).toEqual({ transport: 'rfcomm', mac: '' })
  })

  it('falls back to rfcomm for garbage input with no slash', () => {
    expect(inferTarget('not-a-mac-or-path')).toEqual({
      transport: 'rfcomm',
      mac: 'not-a-mac-or-path',
    })
  })
})

describe('errorMessage', () => {
  it('extracts the message from an Error instance', () => {
    expect(errorMessage(new Error('boom'))).toBe('boom')
  })

  it('extracts the message from an Error subclass', () => {
    class HttpError extends Error {}
    expect(errorMessage(new HttpError('HTTP 400'))).toBe('HTTP 400')
  })

  it('stringifies a plain string', () => {
    expect(errorMessage('plain string')).toBe('plain string')
  })

  it('stringifies non-Error, non-string values', () => {
    expect(errorMessage(42)).toBe('42')
    expect(errorMessage(null)).toBe('null')
    expect(errorMessage(undefined)).toBe('undefined')
    expect(errorMessage({ some: 'object' })).toBe('[object Object]')
  })
})
