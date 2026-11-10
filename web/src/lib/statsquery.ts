// Statistics filters <-> URL query <-> API parameters.

import { PRESETS, presetRange, type PresetKey } from './periods';

export type IncidentsFilter = 'all' | 'active' | 'closed';

export interface StatsFilter {
  preset: PresetKey | 'custom';
  from?: number;
  to?: number;
  location?: number;
  device?: number;
  kind?: string;
  incidents: IncidentsFilter;
}

function posInt(v: string | undefined): number | undefined {
  if (!v || !/^\d+$/.test(v)) return undefined;
  const n = Number(v);
  return n > 0 && Number.isSafeInteger(n) ? n : undefined;
}

export function filterFromQuery(q: Record<string, string>, _now: number): StatsFilter {
  const f: StatsFilter = { preset: '24h', incidents: 'all' };
  const from = posInt(q.from);
  const to = posInt(q.to);
  if ((PRESETS as string[]).includes(q.preset)) f.preset = q.preset as PresetKey;
  else if (from !== undefined && to !== undefined && to > from) Object.assign(f, { preset: 'custom', from, to });
  const location = posInt(q.location);
  const device = posInt(q.device);
  if (location) f.location = location;
  if (device) f.device = device;
  if (q.kind?.trim()) f.kind = q.kind.trim();
  if (q.incidents === 'active' || q.incidents === 'closed') f.incidents = q.incidents;
  return f;
}

export function filterToQuery(f: StatsFilter): Record<string, string> {
  const q: Record<string, string> = {};
  if (f.preset === 'custom') {
    q.from = String(f.from);
    q.to = String(f.to);
  } else if (f.preset !== '24h') q.preset = f.preset;
  if (f.location) q.location = String(f.location);
  if (f.device) q.device = String(f.device);
  if (f.kind) q.kind = f.kind;
  if (f.incidents !== 'all') q.incidents = f.incidents;
  return q;
}

export function range(f: StatsFilter, now: number): { from: number; to: number } {
  if (f.preset === 'custom') return { from: f.from!, to: f.to! };
  return presetRange(f.preset, now);
}

export function apiParams(f: StatsFilter, now: number): Record<string, string> {
  const r = range(f, now);
  const p: Record<string, string> = { from: String(Math.round(r.from)), to: String(Math.round(r.to)) };
  if (f.location) p.location = String(f.location);
  if (f.device) p.device = String(f.device);
  if (f.kind) p.kind = f.kind;
  p.incidents = f.incidents;
  return p;
}
