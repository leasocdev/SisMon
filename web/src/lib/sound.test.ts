import { describe, expect, it } from 'vitest';
import { soundFor } from './sound.svelte';

const on = { on: true, degraded: false };

describe('soundFor', () => {
  it('alarm on a new down', () => {
    expect(soundFor('up', 'down', on)).toBe('alarm');
    expect(soundFor('degraded', 'down', on)).toBe('alarm');
    expect(soundFor(undefined, 'down', on)).toBe('alarm');
  });
  it('recovery from down to up', () => {
    expect(soundFor('down', 'up', on)).toBe('recovery');
  });
  it('degraded only when enabled', () => {
    expect(soundFor('up', 'degraded', on)).toBeNull();
    expect(soundFor('up', 'degraded', { on: true, degraded: true })).toBe('alarm');
  });
  it('no sound for unchanged or unrelated transitions', () => {
    expect(soundFor('down', 'down', on)).toBeNull();
    expect(soundFor('unknown', 'up', on)).toBeNull();
    expect(soundFor('down', undefined, on)).toBeNull();
  });
  it('silent when off', () => {
    expect(soundFor('up', 'down', { on: false, degraded: true })).toBeNull();
  });
});
