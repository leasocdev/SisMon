import { describe, expect, it } from 'vitest';
import { emptyLocation, fieldPath, isDirty, moveDevice, parseCoord, toInput, toPayload } from './editor';
import type { AdminLocation } from './types';

const loc: AdminLocation = {
  id: 3, name: 'Cahul', address: 'str. 1', notes: '', lat: 45.9, lon: 28.19, sort_order: 0,
  devices: [
    { id: 7, location_id: 3, rev: 1, name: 'Router', host: '10.0.1.1', kind: 'Router', is_primary: true, sort_order: 0, overrides: {} },
    { id: 8, location_id: 3, rev: 1, name: 'AP', host: '10.0.1.2', kind: 'Radiolink', is_primary: false, sort_order: 1, overrides: { failures: 5 } },
  ],
};

describe('editor', () => {
  it('fieldPath', () => {
    expect(fieldPath('devices.2.host')).toEqual({ device: 2, name: 'host' });
    expect(fieldPath('devices.0.interval_s')).toEqual({ device: 0, name: 'interval_s' });
    expect(fieldPath('name')).toEqual({ name: 'name' });
  });
  it('moveDevice', () => {
    expect(moveDevice(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a']);
    expect(moveDevice(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b']);
    expect(moveDevice(['a', 'b'], 0, 5)).toEqual(['a', 'b']);
  });
  it('isDirty', () => {
    const x = toInput(loc);
    expect(isDirty(x, structuredClone(x))).toBe(false);
    const y = structuredClone(x);
    y.devices[0].host = '10.0.1.9';
    expect(isDirty(x, y)).toBe(true);
    const z = structuredClone(x);
    z.name = '  Cahul  ';
    expect(isDirty(x, z)).toBe(false);
  });
  it('parseCoord', () => {
    expect(parseCoord('46,99')).toBe(46.99);
    expect(parseCoord(' 28.5 ')).toBe(28.5);
    expect(parseCoord('')).toBeNull();
    expect(parseCoord('abc')).toBeNull();
  });
  it('toInput keeps ids and overrides; emptyLocation has one primary row', () => {
    const x = toInput(loc);
    expect(x.devices.map((d) => d.id)).toEqual([7, 8]);
    expect(x.devices[1].overrides).toEqual({ failures: 5 });
    const e = emptyLocation();
    expect(e.devices).toHaveLength(1);
    expect(e.devices[0].is_primary).toBe(true);
  });
  it('toPayload drops empty overrides', () => {
    const x = toInput(loc);
    x.devices[0].overrides = { interval_s: undefined, failures: 4 };
    const p = toPayload(x);
    expect(p.devices[0].overrides).toEqual({ failures: 4 });
    expect('id' in p.devices[0]).toBe(true);
  });
});
