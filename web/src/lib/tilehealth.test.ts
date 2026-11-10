import { describe, expect, it } from 'vitest';
import { TileHealth } from './tilehealth';

describe('TileHealth', () => {
  it('stays on tiles while they load', () => {
    const h = new TileHealth();
    for (let i = 0; i < 10; i++) h.record(true, 1000 + i);
    expect(h.shouldFallback(2000)).toBe(false);
  });
  it('falls back when most visible tiles fail', () => {
    const h = new TileHealth();
    for (let i = 0; i < 4; i++) h.record(true, 1000 + i);
    for (let i = 0; i < 6; i++) h.record(false, 1100 + i);
    expect(h.shouldFallback(2000)).toBe(true);
  });
  it('needs at least six failures', () => {
    const h = new TileHealth();
    h.record(true, 1000);
    for (let i = 0; i < 3; i++) h.record(false, 1001 + i);
    expect(h.shouldFallback(2000)).toBe(false);
  });
  it('forgets events older than 30 s', () => {
    const h = new TileHealth();
    for (let i = 0; i < 8; i++) h.record(false, 1000 + i);
    expect(h.shouldFallback(2000)).toBe(true);
    expect(h.shouldFallback(1000 + 31_000)).toBe(false);
  });
  it('reset clears history', () => {
    const h = new TileHealth();
    for (let i = 0; i < 8; i++) h.record(false, 1000 + i);
    h.reset();
    expect(h.shouldFallback(2000)).toBe(false);
  });
});
