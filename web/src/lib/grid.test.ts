import { describe, expect, it } from 'vitest';
import { computeGrid } from './grid';

describe('computeGrid', () => {
  it('fits small counts', () => {
    const g = computeGrid(3, 1920, 1080);
    expect(g.cols * g.rows).toBeGreaterThanOrEqual(3);
  });
  it('fits 60 locations on a 1080p screen, wider than tall', () => {
    const g = computeGrid(60, 1920, 1080);
    expect(g.cols * g.rows).toBeGreaterThanOrEqual(60);
    expect(g.cols).toBeGreaterThanOrEqual(g.rows);
  });
  it('handles zero', () => {
    expect(computeGrid(0, 1920, 1080)).toMatchObject({ cols: 1, rows: 1 });
  });
  it('font shrinks with more tiles but stays readable', () => {
    const a = computeGrid(4, 1920, 1080).fontPx;
    const b = computeGrid(60, 1920, 1080).fontPx;
    const c = computeGrid(500, 1920, 1080).fontPx;
    expect(a).toBeGreaterThan(b);
    expect(b).toBeGreaterThanOrEqual(c);
    expect(c).toBeGreaterThanOrEqual(12);
  });
});
