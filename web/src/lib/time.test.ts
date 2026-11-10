import { describe, expect, it } from 'vitest';
import { fmtDateTime, fmtDuration, fmtSince, startOfDayChisinau, utcToZonedParts, zonedToUtc } from './time';

describe('environment', () => {
  it('runs in a foreign browser time zone', () => {
    expect(Intl.DateTimeFormat().resolvedOptions().timeZone).toBe('America/New_York');
  });
});

describe('zonedToUtc', () => {
  it('spring-forward gap moves forward', () => {
    expect(zonedToUtc('2026-03-29', '03:30')).toBe(Date.UTC(2026, 2, 29, 0, 30));
    expect(zonedToUtc('2026-03-29', '02:30')).toBe(Date.UTC(2026, 2, 29, 0, 30));
  });
  it('fall-back ambiguity takes the first occurrence', () => {
    expect(zonedToUtc('2026-10-25', '02:30')).toBe(Date.UTC(2026, 9, 24, 23, 30));
    expect(zonedToUtc('2026-10-25', '03:30')).toBe(Date.UTC(2026, 9, 25, 1, 30));
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
