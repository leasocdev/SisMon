import { describe, expect, it } from 'vitest';
import { cardDevices, errorText, filterLocations, fmtLatency, fmtPct, pendingText, sortLocations, stateText, totals } from './status';
import type { LocationView, State } from './types';

function loc(id: number, name: string, state: State, sort_order = 0): LocationView {
  return { id, name, state, sort_order, address: '', notes: '', lat: null, lon: null, incomplete: false, since: 0, devices: [] };
}

const list = [loc(1, 'Bălți', 'up', 2), loc(2, 'Cahul', 'down', 0), loc(3, 'Anenii', 'unknown', 1), loc(4, 'Orhei', 'degraded', 4), loc(5, 'Edineț', 'error', 3)];

describe('status', () => {
  it('sorts by severity then name', () => {
    expect(sortLocations(list, 'severity').map((l) => l.name)).toEqual(['Cahul', 'Orhei', 'Anenii', 'Edineț', 'Bălți']);
  });
  it('sorts alphabetically with Romanian collation', () => {
    expect(sortLocations(list, 'alpha').map((l) => l.name)).toEqual(['Anenii', 'Bălți', 'Cahul', 'Edineț', 'Orhei']);
  });
  it('sorts manually by sort_order', () => {
    expect(sortLocations(list, 'manual').map((l) => l.id)).toEqual([2, 3, 1, 5, 4]);
  });
  it('does not mutate its input', () => {
    const copy = [...list];
    sortLocations(list, 'alpha');
    expect(list).toEqual(copy);
  });
  it('labels', () => {
    expect(stateText('down')).toBe('✕ Indisponibilă');
    expect(stateText('up')).toBe('✓ Funcțională');
    expect(stateText('degraded')).toBe('▲ Degradată');
    expect(stateText('unknown')).toBe('? Fără date');
    expect(stateText('error')).toBe('! Eroare monitor');
  });
  it('pending text', () => {
    expect(pendingText({ state: 'up', pending: 'down' })).toBe('cădere în confirmare');
    expect(pendingText({ state: 'down', pending: 'up' })).toBe('revenire în confirmare');
    expect(pendingText({ state: 'up', pending: '' })).toBe('');
  });
  it('totals', () => {
    expect(totals(list)).toEqual({ up: 1, down: 1, degraded: 1, unknown: 1, error: 1 });
  });
  it('numbers', () => {
    expect(fmtPct(99.954)).toBe('99,95%');
    expect(fmtPct(0.29)).toBe('0,29%');
    expect(fmtPct(1.13)).toBe('1,13%');
    expect(fmtPct(4.35)).toBe('4,35%');
    expect(fmtPct(100)).toBe('100,00%');
    expect(fmtPct(null)).toBe('—');
    expect(fmtLatency(12.34)).toBe('12,3 ms');
    expect(fmtLatency(null)).toBe('—');
  });
  it('error codes', () => {
    expect(errorText('net_unreachable')).toBe('Rețea/rută inaccesibilă');
    expect(errorText('timeout')).toBe('Fără răspuns');
    expect(errorText('')).toBe('');
  });
});

describe('filterLocations', () => {
  const l = (id: number, name: string, state: State, hosts: string[]) => ({
    ...loc(id, name, state),
    devices: hosts.map((h, i) => ({
      id: id * 10 + i, location_id: id, name: 'Dev' + i, host: h, kind: 'Router', is_primary: i === 0, state, pending: '' as const,
      since: 0, last_check: null, latency_ms: null, loss_pct: null, error_code: '', dns_warning: false,
    })),
  });
  const all = [l(1, 'Chișinău Centru', 'up', ['10.0.0.1']), l(2, 'Cahul', 'down', ['10.0.1.1']), l(3, 'Bălți', 'degraded', ['10.0.2.2'])];
  it('by state set', () => {
    expect(filterLocations(all, new Set(['down']), '').map((x) => x.id)).toEqual([2]);
    expect(filterLocations(all, new Set(), '').map((x) => x.id)).toEqual([1, 2, 3]);
  });
  it('by text: name without diacritics, device host', () => {
    expect(filterLocations(all, new Set(), 'chisinau').map((x) => x.id)).toEqual([1]);
    expect(filterLocations(all, new Set(), '10.0.2.2').map((x) => x.id)).toEqual([3]);
    expect(filterLocations(all, new Set(), 'BALTI').map((x) => x.id)).toEqual([3]);
  });
});

describe('cardDevices', () => {
  const dev = (name: string, state: State, is_primary = false, pending: State | '' = '') => ({ name, state, is_primary, pending });

  it('problemele primele, restul în ordinea configurată', () => {
    const list = [dev('a', 'up'), dev('b', 'up', false, 'down'), dev('c', 'down'), dev('d', 'up'), dev('e', 'unknown')];
    expect(cardDevices('down', list, false).shown.map((d) => d.name)).toEqual(['c', 'e', 'b', 'a', 'd']);
  });

  it('modul compact: locație funcțională → doar primarele și cele cu probleme', () => {
    const list = [dev('a', 'up', true), dev('b', 'up'), dev('c', 'up', false, 'down'), dev('d', 'up')];
    const r = cardDevices('up', list, true);
    expect(r.shown.map((d) => d.name)).toEqual(['c', 'a']);
    expect(r.hidden).toBe(2);
  });

  it('modul compact nu atinge locațiile cu probleme', () => {
    const list = [dev('a', 'up', true), dev('b', 'down')];
    expect(cardDevices('degraded', list, true)).toEqual({ shown: [list[1], list[0]], hidden: 0 });
  });

  it('modul compact fără primare → toate echipamentele', () => {
    const list = [dev('a', 'up'), dev('b', 'up')];
    expect(cardDevices('up', list, true)).toEqual({ shown: list, hidden: 0 });
  });
});
