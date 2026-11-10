// Dispatch screen grid: the column count that gives the largest tiles of at
// least `minAspect` (width/height) while all n tiles fit without scrolling.

export interface Grid {
  cols: number;
  rows: number;
  fontPx: number;
}

export function computeGrid(n: number, width: number, height: number, minAspect = 1.6): Grid {
  if (n <= 0 || width <= 0 || height <= 0) return { cols: 1, rows: 1, fontPx: 28 };
  let best = { cols: 1, rows: n, score: -1 };
  for (let cols = 1; cols <= n; cols++) {
    const rows = Math.ceil(n / cols);
    const cw = width / cols;
    const ch = height / rows;
    const score = Math.min(cw, ch * minAspect); // usable tile width at the target aspect
    if (score > best.score) best = { cols, rows, score };
  }
  const tileH = height / best.rows;
  const tileW = width / best.cols;
  const fontPx = Math.round(Math.max(12, Math.min(30, tileH / 6, tileW / 11)));
  return { cols: best.cols, rows: best.rows, fontPx };
}
