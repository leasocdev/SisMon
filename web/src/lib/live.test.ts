import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { applyEvent, healthWarning, LiveStore, type LiveDeps } from './live.svelte';
import { fmtSince } from './time';
import type { Health, LocationView, StateResponse } from './types';

function loc(id: number, state: LocationView['state'] = 'up', name = 'L' + id): LocationView {
  return { id, name, state, sort_order: 0, address: '', notes: '', lat: null, lon: null, incomplete: false, since: 0, devices: [] };
}

const health: Health = {
  last_cycle: null, late_checks: 0, avg_check_ms: 0, icmp_mode: 'fake', errors: [], db_size_bytes: 0,
  last_backup: null, backup_error: '', warning: '',
};

describe('applyEvent', () => {
  const base = { locations: [loc(1), loc(2), loc(3)], epoch: 10, lastId: '10-5' };
  it('snapshot replaces the list (missing locations disappear)', () => {
    const r = applyEvent(base, 'snapshot', { epoch: 10, seq: 6, locations: [loc(1), loc(2)], health }, '10-6');
    expect(r.locations.map((l) => l.id)).toEqual([1, 2]);
    expect(r.lastId).toBe('10-6');
  });
  it('snapshot with a new epoch is adopted', () => {
    const r = applyEvent(base, 'snapshot', { epoch: 99, seq: 1, locations: [loc(7)], health }, '99-1');
    expect(r.epoch).toBe(99);
    expect(r.locations.map((l) => l.id)).toEqual([7]);
  });
  it('state upserts', () => {
    const r = applyEvent(base, 'state', loc(4, 'down'), '10-6');
    expect(r.locations.find((l) => l.id === 4)?.state).toBe('down');
    const r2 = applyEvent(r, 'state', loc(1, 'down'), '10-7');
    expect(r2.locations.find((l) => l.id === 1)?.state).toBe('down');
    expect(r2.locations).toHaveLength(4);
  });
  it('removed deletes', () => {
    const r = applyEvent(base, 'removed', 2, '10-6');
    expect(r.locations.map((l) => l.id)).toEqual([1, 3]);
  });
});

describe('healthWarning', () => {
  it('uses the server warning when present', () => {
    expect(healthWarning({ ...health, warning: 'X' }, 60, 1_000_000)).toBe('X');
  });
  it('warns when the last cycle is older than two intervals', () => {
    expect(healthWarning({ ...health, last_cycle: 1_000_000 - 3 * 60_000 }, 60, 1_000_000)).toBe('Monitorul nu a scris date recent.');
    expect(healthWarning({ ...health, last_cycle: 1_000_000 - 30_000 }, 60, 1_000_000)).toBe('');
  });
  it('no cycle yet is not a warning during the first two intervals', () => {
    expect(healthWarning({ ...health, last_cycle: null }, 60, 1_000_000, 1_000_000 - 10_000)).toBe('');
    expect(healthWarning({ ...health, last_cycle: null }, 60, 1_000_000, 1_000_000 - 200_000)).toBe('Monitorul nu a scris date recent.');
  });
});

class FakeES {
  static instances: FakeES[] = [];
  static CLOSED = 2;
  url: string;
  readyState = 0;
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;
  constructor(url: string) {
    this.url = url;
    FakeES.instances.push(this);
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(fn);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  emit(type: string, data: unknown, id = '') {
    for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data), lastEventId: id } as MessageEvent);
  }
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  fail() {
    this.readyState = 2;
    this.onerror?.();
  }
}

describe('LiveStore', () => {
  let localNow = 1_000_000;
  const state: StateResponse = {
    epoch: 10, seq: 5, locations: [loc(1), loc(2)], health,
    identity: { name: 'SisMon', subtitle: '', report_title: 'R', logo_version: 0 }, interval_s: 60, version: 't', now: 1_060_000, demo: false,
  };
  function deps(): LiveDeps {
    return {
      EventSource: FakeES as unknown as LiveDeps['EventSource'],
      fetch: vi.fn(async () => new Response(JSON.stringify(state), { headers: { 'Content-Type': 'application/json' } })) as unknown as typeof fetch,
      now: () => localNow,
      reload: () => {},
    };
  }
  beforeEach(() => {
    vi.useFakeTimers();
    FakeES.instances = [];
    localNow = 1_000_000;
  });
  afterEach(() => vi.useRealTimers());

  it('loads state, then subscribes after the snapshot sequence', async () => {
    const s = new LiveStore(deps());
    await s.start();
    expect(s.locations).toHaveLength(2);
    expect(s.identity.name).toBe('SisMon');
    expect(FakeES.instances[0].url).toBe('/api/events?last=10-5');
    expect(s.clockOffset).toBe(60_000);
    expect(fmtSince(s.serverNow() - 60_000, s.serverNow())).toBe('de 1 min');
    s.stop();
  });

  it('reconnects with the last event id after the stream closes', async () => {
    const s = new LiveStore(deps());
    await s.start();
    const es = FakeES.instances[0];
    es.open();
    expect(s.connected).toBe(true);
    es.emit('state', loc(1, 'down'), '10-6');
    es.fail();
    expect(s.connected).toBe(false);
    await vi.advanceTimersByTimeAsync(2100);
    expect(FakeES.instances).toHaveLength(2);
    expect(FakeES.instances[1].url).toBe('/api/events?last=10-6');
    s.stop();
  });

  it('marks data stale after 30 s without messages and recovers on ping', async () => {
    const s = new LiveStore(deps());
    await s.start();
    const es = FakeES.instances[0];
    es.open();
    localNow += 31_000;
    await vi.advanceTimersByTimeAsync(1100);
    expect(s.stale).toBe(true);
    es.emit('ping', { now: 2_000_000 });
    await vi.advanceTimersByTimeAsync(1100);
    expect(s.stale).toBe(false);
    expect(s.clockOffset).toBe(2_000_000 - localNow);
    s.stop();
  });

  it('notifies changes but not the initial load, and diffs snapshots', async () => {
    const s = new LiveStore(deps());
    const seen: string[] = [];
    s.onChange((prev, next) => seen.push(`${prev?.id ?? '-'}:${prev?.state ?? '-'}>${next?.state ?? '-'}`));
    await s.start();
    expect(seen).toEqual([]);
    const es = FakeES.instances[0];
    es.emit('state', loc(1, 'down'), '10-6');
    es.emit('snapshot', { epoch: 10, seq: 7, locations: [loc(1, 'down'), loc(3, 'up')], health }, '10-7');
    expect(seen).toEqual(['1:up>down', '2:up>-', '-:->up']);
    s.stop();
  });
});

describe('LiveStore after a server restart', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeES.instances = [];
  });
  afterEach(() => vi.useRealTimers());

  function mk(version: string, name: string, epoch: number): StateResponse {
    return {
      epoch, seq: 1, locations: [loc(1)], health,
      identity: { name, subtitle: '', report_title: 'R', logo_version: 0 }, interval_s: 60, version, now: 1000, demo: false,
    };
  }

  it('refetches /api/state on a new epoch and reloads when the version changed', async () => {
    const responses = [mk('1.0', 'A', 10), mk('1.0', 'B', 20), mk('1.1', 'B', 30)];
    const fetchFn = vi.fn(async () => new Response(JSON.stringify(responses.shift()), { headers: { 'Content-Type': 'application/json' } }));
    const reload = vi.fn();
    const s = new LiveStore({ EventSource: FakeES as unknown as LiveDeps['EventSource'], fetch: fetchFn as unknown as typeof fetch, now: () => 1000, reload });
    await s.start();
    const es = FakeES.instances[0];
    es.emit('snapshot', { epoch: 20, seq: 1, locations: [loc(1)], health }, '20-1');
    await vi.advanceTimersByTimeAsync(10);
    expect(fetchFn).toHaveBeenCalledTimes(2);
    expect(s.identity.name).toBe('B');
    expect(reload).not.toHaveBeenCalled();
    es.emit('snapshot', { epoch: 30, seq: 1, locations: [loc(1)], health }, '30-1');
    await vi.advanceTimersByTimeAsync(10);
    expect(reload).toHaveBeenCalledTimes(1);
    s.stop();
  });

  it('first snapshot after a failed initial load does not notify', async () => {
    const fetchFn = vi.fn(async () => new Response('x', { status: 502 }));
    const s = new LiveStore({ EventSource: FakeES as unknown as LiveDeps['EventSource'], fetch: fetchFn as unknown as typeof fetch, now: () => 1000 });
    const seen: number[] = [];
    s.onChange((_p, n) => n && seen.push(n.id));
    await s.start();
    FakeES.instances[0].emit('snapshot', { epoch: 5, seq: 1, locations: [loc(1, 'down'), loc(2, 'down')], health }, '5-1');
    expect(seen).toEqual([]);
    expect(s.locations).toHaveLength(2);
    s.stop();
  });
});
