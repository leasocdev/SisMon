// Live state: one SSE stream for the whole app.

import type { Health, Identity, LocationView, Snapshot, StateResponse } from './types';

export interface LiveData {
  locations: LocationView[];
  epoch: number;
  lastId: string;
}

/** Pure reducer for one SSE event. */
export function applyEvent(st: LiveData, type: string, data: unknown, id: string): LiveData {
  const lastId = id || st.lastId;
  switch (type) {
    case 'snapshot': {
      const s = data as Snapshot;
      return { locations: s.locations, epoch: s.epoch, lastId };
    }
    case 'state': {
      const l = data as LocationView;
      const i = st.locations.findIndex((x) => x.id === l.id);
      const locations = [...st.locations];
      if (i >= 0) locations[i] = l;
      else locations.push(l);
      return { ...st, locations, lastId };
    }
    case 'removed': {
      const rid = data as number;
      return { ...st, locations: st.locations.filter((x) => x.id !== rid), lastId };
    }
  }
  return { ...st, lastId };
}

export const STALE_MS = 30_000;

/** Monitor warning: the server's own, or a missing/old last cycle (> 2 intervals). */
export function healthWarning(h: Health | null, intervalS: number, serverNow: number, startedAt = serverNow): string {
  if (!h) return '';
  if (h.warning) return h.warning;
  const limit = 2 * intervalS * 1000;
  const ref = h.last_cycle ?? startedAt;
  return serverNow - ref > limit ? 'Monitorul nu a scris date recent.' : '';
}

type ESLike = {
  readyState: number;
  onopen: (() => void) | null;
  onerror: (() => void) | null;
  addEventListener(type: string, fn: (e: MessageEvent) => void): void;
  close(): void;
};

export interface LiveDeps {
  EventSource: new (url: string) => ESLike;
  fetch: typeof fetch;
  now: () => number;
  /** Reloads the page (after a server upgrade, to fetch the new UI). */
  reload: () => void;
}

type ChangeFn = (prev: LocationView | undefined, next: LocationView | undefined) => void;

const EMPTY_HEALTH: Health = {
  last_cycle: null, late_checks: 0, avg_check_ms: 0, icmp_mode: '', errors: [], db_size_bytes: 0,
  last_backup: null, backup_error: '', warning: '',
};

export class LiveStore {
  locations = $state<LocationView[]>([]);
  health = $state<Health>(EMPTY_HEALTH);
  identity = $state<Identity>({ name: 'NetWatch', subtitle: 'NETWORK MONITOR', report_title: 'Raport de disponibilitate', logo_version: 0 });
  intervalS = $state(60);
  version = $state('');
  demo = $state(false);
  epoch = $state(0);
  connected = $state(false);
  stale = $state(false);
  loaded = $state(false);
  loadError = $state('');
  clockOffset = $state(0);
  /** Local clock ticking every second, for "de 12 min" displays. */
  tick = $state(0);

  private lastId = '';
  private lastMessageAt = 0;
  private startedAt = 0;
  private es: ESLike | null = null;
  private timer: ReturnType<typeof setInterval> | null = null;
  private retry: ReturnType<typeof setTimeout> | null = null;
  private backoff = 2000;
  private listeners = new Set<ChangeFn>();
  private deps: LiveDeps;
  private running = false;

  constructor(deps?: Partial<LiveDeps>) {
    this.deps = {
      EventSource: deps?.EventSource ?? (globalThis.EventSource as unknown as LiveDeps['EventSource']),
      fetch: deps?.fetch ?? ((...a: Parameters<typeof fetch>) => fetch(...a)),
      now: deps?.now ?? (() => Date.now()),
      reload: deps?.reload ?? (() => location.reload()),
    };
  }

  serverNow(): number {
    return this.deps.now() + this.clockOffset;
  }

  location(id: number): LocationView | undefined {
    return this.locations.find((l) => l.id === id);
  }

  warning(): string {
    return healthWarning(this.health, this.intervalS, this.serverNow(), this.startedAt);
  }

  onChange(fn: ChangeFn): () => void {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  }

  async start(): Promise<void> {
    if (this.running) return;
    this.running = true;
    this.lastMessageAt = this.deps.now();
    this.timer = setInterval(() => {
      this.tick++;
      this.stale = this.deps.now() - this.lastMessageAt > STALE_MS;
    }, 1000);
    try {
      const s = await this.fetchState();
      this.startedAt = s.now;
      this.health = s.health;
      this.set({ locations: s.locations, epoch: s.epoch, lastId: `${s.epoch}-${s.seq}` }, false);
      this.loaded = true;
      this.loadError = '';
    } catch {
      this.loadError = 'Serverul nu răspunde. Se reîncearcă…';
      this.startedAt = this.serverNow();
    }
    this.connect();
  }

  stop(): void {
    this.running = false;
    this.es?.close();
    this.es = null;
    if (this.timer) clearInterval(this.timer);
    if (this.retry) clearTimeout(this.retry);
    this.timer = this.retry = null;
  }

  /** Identity, interval and version; reloads the page when the server version changed. */
  private async fetchState(): Promise<StateResponse> {
    const res = await this.deps.fetch('/api/state', { credentials: 'same-origin' });
    if (!res.ok) throw new Error(String(res.status));
    const s = (await res.json()) as StateResponse;
    if (this.version && s.version !== this.version) {
      this.deps.reload();
    }
    this.identity = s.identity;
    this.intervalS = s.interval_s;
    this.version = s.version;
    this.demo = !!s.demo;
    this.clockOffset = s.now - this.deps.now();
    return s;
  }

  private set(next: LiveData, notify: boolean): void {
    const prev = this.locations;
    this.locations = next.locations;
    this.epoch = next.epoch;
    this.lastId = next.lastId;
    if (!notify || this.listeners.size === 0) return;
    const before = new Map(prev.map((l) => [l.id, l]));
    const after = new Map(next.locations.map((l) => [l.id, l]));
    const changes: [LocationView | undefined, LocationView | undefined][] = [];
    for (const [id, l] of before) if (!after.has(id)) changes.push([l, undefined]);
    for (const [id, l] of after) {
      const p = before.get(id);
      if (!p || p.state !== l.state) changes.push([p, l]);
    }
    for (const [p, n] of changes) for (const fn of this.listeners) fn(p, n);
  }

  private connect(): void {
    if (!this.running) return;
    const url = this.lastId ? `/api/events?last=${encodeURIComponent(this.lastId)}` : '/api/events';
    const es = new this.deps.EventSource(url);
    this.es = es;
    const seen = () => {
      this.lastMessageAt = this.deps.now();
      this.stale = false;
    };
    const handle = (type: string) => (e: MessageEvent) => {
      seen();
      this.connected = true;
      let data: unknown;
      try {
        data = JSON.parse(e.data);
      } catch {
        return;
      }
      if (type === 'ping') {
        const now = (data as { now?: number }).now;
        if (typeof now === 'number') this.clockOffset = now - this.deps.now();
        return;
      }
      if (type === 'health') {
        this.health = data as Health;
        this.lastId = e.lastEventId || this.lastId;
        return;
      }
      let notify = true;
      if (type === 'snapshot') {
        const snap = data as Snapshot;
        this.health = snap.health;
        notify = this.loaded; // nothing shown yet: no alarms for the initial picture
        if (this.epoch && snap.epoch !== this.epoch) this.fetchState().catch(() => {}); // server restarted
        this.loaded = true;
        this.loadError = '';
      }
      this.set(applyEvent({ locations: this.locations, epoch: this.epoch, lastId: this.lastId }, type, data, e.lastEventId), notify);
    };
    for (const t of ['snapshot', 'state', 'removed', 'health', 'ping']) es.addEventListener(t, handle(t));
    es.onopen = () => {
      seen();
      this.connected = true;
      this.backoff = 2000;
    };
    es.onerror = () => {
      this.connected = false;
      if (es.readyState !== 2) return; // the browser retries by itself
      es.close();
      if (!this.running) return;
      const wait = this.backoff;
      this.backoff = Math.min(this.backoff * 2, 30_000);
      this.retry = setTimeout(() => this.connect(), wait);
    };
  }
}

export const live = new LiveStore();
