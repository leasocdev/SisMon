import { describe, expect, it } from 'vitest';
import { presetRange } from './periods';
import { zonedToUtc } from './time';

describe('presetRange', () => {
  const n = Date.UTC(2026, 2, 15, 12);
  it('24h, 7z, 30z', () => {
    expect(presetRange('24h', n)).toEqual({ from: n - 86_400_000, to: n });
    expect(presetRange('7z', n)).toEqual({ from: n - 7 * 86_400_000, to: n });
    expect(presetRange('30z', n)).toEqual({ from: n - 30 * 86_400_000, to: n });
  });
  it('current month starts at local midnight on the 1st', () => {
    expect(presetRange('luna', n)).toEqual({ from: zonedToUtc('2026-03-01', '00:00'), to: n });
  });
  it('previous month is the whole calendar month', () => {
    expect(presetRange('luna-trecuta', n)).toEqual({ from: zonedToUtc('2026-02-01', '00:00'), to: zonedToUtc('2026-03-01', '00:00') });
  });
  it('previous month across a year boundary', () => {
    expect(presetRange('luna-trecuta', Date.UTC(2026, 0, 10))).toEqual({ from: zonedToUtc('2025-12-01', '00:00'), to: zonedToUtc('2026-01-01', '00:00') });
  });
});
