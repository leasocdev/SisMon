import { describe, expect, it } from 'vitest';
import { fmtDateTime, fmtDuration, fmtSince, startOfDayChisinau, utcToZonedParts, zonedToUtc } from './time';

describe('environment', () => {
  it('runs in a foreign browser time zone', () => {
    expect(Intl.DateTimeFormat().resolvedOptions().timeZone).toBe('America/New_York');
  });
});

const chisinau = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Europe/Chisinau',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  hourCycle: 'h23',
});

/** Chisinau wall date ('YYYY-MM-DD') and hour at instant ms. */
function wallOf(ms: number): { date: string; h: number } {
  const p = Object.fromEntries(chisinau.formatToParts(ms).map((x) => [x.type, x.value]));
  return { date: `${p.year}-${p.month}-${p.day}`, h: Number(p.hour) };
}

/** UTC offset of Chisinau at instant ms, in minutes. */
function offsetMin(ms: number): number {
  const p = Object.fromEntries(chisinau.formatToParts(ms).map((x) => [x.type, x.value]));
  const minute = new Date(ms).getUTCMinutes();
  return Math.round((Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour, minute) - Math.floor(ms / 60_000) * 60_000) / 60_000);
}

/** The DST switch instant (whole UTC hour) during the UTC day starting at dayStart. */
function transition(dayStart: number): number {
  for (let t = dayStart - 6 * 3_600_000; t < dayStart + 30 * 3_600_000; t += 3_600_000) {
    if (offsetMin(t) !== offsetMin(t - 3_600_000)) return t;
  }
  throw new Error('no DST transition found');
}

const hh = (h: number) => `${String(h).padStart(2, '0')}:30`;

describe('zonedToUtc', () => {
  // The transition hour comes from the engine's tz data, not from the test:
  // tzdata 2025b has Moldova switching at 02:00 local, newer releases at the EU
  // time (03:00 local), so CI and a developer machine can disagree.
  it('spring-forward gap moves forward', () => {
    const t = transition(Date.UTC(2026, 2, 29));
    const last = wallOf(t - 1000); // last second before the jump: H:59:59 winter time
    // (H+1):30 does not exist (skipped hour) → moves forward to the same moment as (H+2):30.
    expect(zonedToUtc(last.date, hh(last.h + 1))).toBe(t + 30 * 60_000);
    // (H+2):30 is plain summer time, 30 min after the jump.
    expect(zonedToUtc(last.date, hh(last.h + 2))).toBe(t + 30 * 60_000);
  });
  it('fall-back ambiguity takes the first occurrence', () => {
    const t = transition(Date.UTC(2026, 9, 25));
    const rep = wallOf(t); // first instant of winter time: H:00, an hour that happens twice
    // H:30 happens twice; the first one is still summer time, 30 min before the switch.
    expect(zonedToUtc(rep.date, hh(rep.h))).toBe(t - 30 * 60_000);
    // (H+1):30 is unambiguous winter time.
    expect(zonedToUtc(rep.date, hh(rep.h + 1))).toBe(t + 90 * 60_000);
  });
  it('summer and winter offsets', () => {
    expect(zonedToUtc('2026-07-01', '00:00')).toBe(Date.UTC(2026, 5, 30, 21, 0));
    expect(zonedToUtc('2026-01-15', '12:00')).toBe(Date.UTC(2026, 0, 15, 10, 0));
  });
  it('rejects malformed input', () => {
    expect(Number.isNaN(zonedToUtc('2026-13-01', '00:00'))).toBe(true);
    expect(Number.isNaN(zonedToUtc('', '10:00'))).toBe(true);
  });
});

describe('formatting', () => {
  it('utcToZonedParts', () => {
    expect(utcToZonedParts(Date.UTC(2026, 5, 30, 21, 0))).toEqual({ date: '2026-07-01', time: '00:00' });
  });
  it('fmtDateTime in Chisinau', () => {
    expect(fmtDateTime(Date.UTC(2026, 10, 9, 12, 5))).toBe('09.11.2026 14:05');
  });
  it('fmtDuration', () => {
    expect(fmtDuration(45)).toBe('45 s');
    expect(fmtDuration(720)).toBe('12 min');
    expect(fmtDuration(11100)).toBe('3 h 05 min');
    expect(fmtDuration(187200)).toBe('2 z 4 h');
    expect(fmtDuration(0)).toBe('0 s');
  });
  it('fmtSince clamps negatives', () => {
    expect(fmtSince(1000, 500)).toBe('de 0 s');
    expect(fmtSince(0, 720_000)).toBe('de 12 min');
  });
  it('startOfDayChisinau', () => {
    expect(startOfDayChisinau(Date.UTC(2026, 5, 30, 22, 0))).toBe(Date.UTC(2026, 5, 30, 21, 0));
  });
});
