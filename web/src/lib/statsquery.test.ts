import { describe, expect, it } from 'vitest';
import { apiParams, filterFromQuery, filterToQuery } from './statsquery';

const now = Date.UTC(2026, 10, 9, 12);
const q = (s: string) => Object.fromEntries(new URLSearchParams(s));

describe('statsquery', () => {
  it('reads presets and ids', () => {
    expect(filterFromQuery(q('preset=7z&location=3'), now)).toEqual({ preset: '7z', location: 3, incidents: 'all' });
  });
  it('reads the archived switch', () => {
    expect(filterFromQuery(q('archived=1'), now)).toEqual({ preset: '24h', incidents: 'all', archived: true });
    expect(filterFromQuery(q('archived=0'), now)).toEqual({ preset: '24h', incidents: 'all' });
  });
  it('falls back to defaults on invalid values', () => {
    expect(filterFromQuery(q('preset=xyz&location=-1&incidents=abc&device=x'), now)).toEqual({ preset: '24h', incidents: 'all' });
  });
  it('custom period requires both ends', () => {
    expect(filterFromQuery(q('from=1000&to=2000'), now)).toEqual({ preset: 'custom', from: 1000, to: 2000, incidents: 'all' });
    expect(filterFromQuery(q('from=1000'), now)).toEqual({ preset: '24h', incidents: 'all' });
  });
  it('round-trips', () => {
    for (const s of ['preset=30z&kind=Router&incidents=active', 'from=1000&to=2000&device=4', 'preset=luna-trecuta&location=2', 'archived=1&location=5', 'degraded=1&preset=7z']) {
      const f = filterFromQuery(q(s), now);
      expect(filterFromQuery(filterToQuery(f), now)).toEqual(f);
    }
    expect(filterToQuery({ preset: '24h', incidents: 'all' })).toEqual({});
  });
  it('builds API parameters', () => {
    expect(apiParams({ preset: 'custom', from: 1, to: 2, location: 3, incidents: 'closed' }, now)).toEqual({
      from: '1', to: '2', location: '3', incidents: 'closed',
    });
    expect(apiParams({ preset: 'custom', from: 1, to: 2, incidents: 'all', archived: true }, now)).toEqual({
      from: '1', to: '2', incidents: 'all', archived: '1',
    });
    expect(apiParams({ preset: '24h', incidents: 'all', kind: 'Router' }, now)).toEqual({
      from: String(now - 86_400_000), to: String(now), incidents: 'all', kind: 'Router',
    });
  });
});
