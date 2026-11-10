import type { LocationView, State } from './types';

export const STATES: State[] = ['down', 'degraded', 'unknown', 'error', 'up'];

export const STATE_LABEL: Record<State, string> = {
  up: 'Funcțională',
  degraded: 'Degradată',
  down: 'Indisponibilă',
  unknown: 'Fără date',
  error: 'Eroare monitor',
};

/** Plural labels for the totals bar. */
export const STATE_PLURAL: Record<State, string> = {
  up: 'Funcționale',
  degraded: 'Degradate',
  down: 'Indisponibile',
  unknown: 'Fără date',
  error: 'Eroare monitor',
};

export const STATE_SYMBOL: Record<State, string> = {
  up: '✓',
  degraded: '▲',
  down: '✕',
  unknown: '?',
  error: '!',
};

export function stateText(s: State): string {
  return `${STATE_SYMBOL[s]} ${STATE_LABEL[s]}`;
}

export function severity(s: State): number {
  switch (s) {
    case 'down':
      return 0;
    case 'degraded':
      return 1;
    case 'unknown':
    case 'error':
      return 2;
  }
  return 3;
}

export type SortMode = 'severity' | 'alpha' | 'manual';

const collator = new Intl.Collator('ro', { sensitivity: 'base' });

export function sortLocations<T extends Pick<LocationView, 'name' | 'state' | 'sort_order'>>(list: T[], mode: SortMode): T[] {
  const byName = (a: T, b: T) => collator.compare(a.name, b.name);
  const out = [...list];
  if (mode === 'alpha') return out.sort(byName);
  if (mode === 'manual') return out.sort((a, b) => a.sort_order - b.sort_order || byName(a, b));
  return out.sort((a, b) => severity(a.state) - severity(b.state) || byName(a, b));
}

export function pendingText(d: { state: State; pending: State | '' }): string {
  if (d.pending === 'down' && d.state !== 'down') return 'cădere în confirmare';
  if (d.pending === 'up' && d.state !== 'up') return 'revenire în confirmare';
  return '';
}

export function totals(list: Pick<LocationView, 'state'>[]): Record<State, number> {
  const t: Record<State, number> = { up: 0, down: 0, degraded: 0, unknown: 0, error: 0 };
  for (const l of list) t[l.state]++;
  return t;
}

const pctFmt = new Intl.NumberFormat('ro-RO', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const msFmt = new Intl.NumberFormat('ro-RO', { minimumFractionDigits: 1, maximumFractionDigits: 1 });

export function fmtPct(v: number | null | undefined): string {
  return v == null ? '—' : pctFmt.format(Math.floor(v * 100 + 1e-9) / 100) + '%';
}

export function fmtLatency(v: number | null | undefined): string {
  return v == null ? '—' : msFmt.format(v) + ' ms';
}

export function fmtLoss(v: number | null | undefined): string {
  return v == null ? '—' : Math.round(v) + '%';
}

const ERROR_TEXT: Record<string, string> = {
  timeout: 'Fără răspuns',
  host_unreachable: 'Echipament inaccesibil',
  net_unreachable: 'Rețea/rută inaccesibilă',
  dns: 'Eroare DNS',
  permission: 'Lipsă permisiune ICMP',
  internal: 'Eroare internă a monitorului',
};

export function errorText(code: string): string {
  return code ? (ERROR_TEXT[code] ?? code) : '';
}

/** Lower-case text without diacritics, for searching. */
export function fold(s: string): string {
  return s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}

/** Keeps locations whose state is in `states` (all if empty) and that match `q`
 * on the location name, a device name or a device host. */
export function filterLocations<T extends Pick<LocationView, 'name' | 'state' | 'devices'>>(list: T[], states: Set<string>, q: string): T[] {
  const needle = fold(q.trim());
  return list.filter((l) => {
    if (states.size && !states.has(l.state)) return false;
    if (!needle) return true;
    if (fold(l.name).includes(needle)) return true;
    return l.devices.some((d) => fold(d.name).includes(needle) || d.host.toLowerCase().includes(needle));
  });
}
