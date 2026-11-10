// All times are shown in Europe/Chisinau, whatever the browser's time zone.

export const TZ = 'Europe/Chisinau';

const partsFmt = new Intl.DateTimeFormat('en-GB', {
  timeZone: TZ,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

interface Wall {
  y: number;
  m: number;
  d: number;
  h: number;
  mi: number;
  s: number;
}

function wall(ms: number): Wall {
  const p: Record<string, number> = {};
  for (const part of partsFmt.formatToParts(ms)) {
    if (part.type !== 'literal') p[part.type] = Number(part.value);
  }
  return { y: p.year, m: p.month, d: p.day, h: p.hour, mi: p.minute, s: p.second };
}

/** Offset of Chisinau from UTC at instant ms, in ms. */
function offset(ms: number): number {
  const w = wall(ms);
  const asUTC = Date.UTC(w.y, w.m - 1, w.d, w.h, w.mi, w.s);
  return asUTC - Math.floor(ms / 1000) * 1000;
}

const pad = (n: number) => String(n).padStart(2, '0');

/** Local Chisinau date ('YYYY-MM-DD') and time ('HH:MM') → UTC ms; NaN if invalid.
 * A time skipped by the spring change moves forward; a repeated time takes its first occurrence. */
export function zonedToUtc(date: string, time: string): number {
  const dm = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const tm = /^(\d{2}):(\d{2})$/.exec(time);
  if (!dm || !tm) return NaN;
  const [y, m, d, h, mi] = [+dm[1], +dm[2], +dm[3], +tm[1], +tm[2]];
  if (m < 1 || m > 12 || d < 1 || h > 23 || mi > 59) return NaN;
  const guess = Date.UTC(y, m - 1, d, h, mi);
  if (new Date(guess).getUTCDate() !== d) return NaN;
  const day = 86_400_000;
  const before = offset(guess - day);
  const after = offset(guess + day);
  const candidates = [...new Set([before, after])]
    .map((o) => guess - o)
    .filter((t) => {
      const w = wall(t);
      return w.y === y && w.m === m && w.d === d && w.h === h && w.mi === mi;
    });
  if (candidates.length) return Math.min(...candidates);
  return guess - before; // inside the spring gap
}

export function utcToZonedParts(ms: number): { date: string; time: string } {
  const w = wall(ms);
  return { date: `${w.y}-${pad(w.m)}-${pad(w.d)}`, time: `${pad(w.h)}:${pad(w.mi)}` };
}

export function startOfDayChisinau(ms: number): number {
  return zonedToUtc(utcToZonedParts(ms).date, '00:00');
}

export function fmtDate(ms: number): string {
  const w = wall(ms);
  return `${pad(w.d)}.${pad(w.m)}.${w.y}`;
}

export function fmtTime(ms: number, seconds = false): string {
  const w = wall(ms);
  return seconds ? `${pad(w.h)}:${pad(w.mi)}:${pad(w.s)}` : `${pad(w.h)}:${pad(w.mi)}`;
}

export function fmtDateTime(ms: number): string {
  return `${fmtDate(ms)} ${fmtTime(ms)}`;
}

/** Short day+time for axes and tooltips: '09.11 14:05'. */
export function fmtShort(ms: number): string {
  const w = wall(ms);
  return `${pad(w.d)}.${pad(w.m)} ${pad(w.h)}:${pad(w.mi)}`;
}

export function fmtDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  if (s < 60) return `${s} s`;
  const min = Math.floor(s / 60);
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h} h ${pad(min % 60)} min`;
  const days = Math.floor(h / 24);
  return `${days} z ${h % 24} h`;
}

/** 'de 12 min' — never negative, even with a skewed clock. */
export function fmtSince(sinceMs: number, nowMs: number): string {
  return 'de ' + fmtDuration((nowMs - sinceMs) / 1000);
}
