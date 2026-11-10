<script lang="ts">
  import { onMount } from 'svelte';
  import type uPlotType from 'uplot';
  import { fmtShort } from '../lib/time';
  import type { Point } from '../lib/types';

  let { points, from, to }: { points: Point[]; from: number; to: number } = $props();

  let host: HTMLDivElement;
  let width = $state(600);
  let UPlot: typeof uPlotType | null = $state(null);
  let theme = $state(document.documentElement.dataset.theme ?? 'light');
  let chart: uPlotType | null = null;

  const css = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

  onMount(() => {
    let alive = true;
    Promise.all([import('uplot'), import('uplot/dist/uPlot.min.css')]).then(([m]) => {
      if (alive) UPlot = m.default;
    });
    const ro = new ResizeObserver(() => (width = Math.max(240, host.clientWidth)));
    ro.observe(host);
    const mo = new MutationObserver(() => (theme = document.documentElement.dataset.theme ?? 'light'));
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    return () => {
      alive = false;
      ro.disconnect();
      mo.disconnect();
      chart?.destroy();
      chart = null;
    };
  });

  $effect(() => {
    if (!UPlot) return;
    void theme;
    const xs = points.map((p) => p.t / 1000);
    const data: uPlotType.AlignedData = [
      xs,
      points.map((p) => p.avg),
      points.map((p) => p.min),
      points.map((p) => p.max),
      points.map((p) => p.loss_pct),
    ];
    const fg = css('--muted');
    const grid = css('--border');
    const line = css('--focus');
    const loss = css('--c-down');
    chart?.destroy();
    chart = new UPlot(
      {
        width,
        height: 220,
        scales: { x: { time: true, min: from / 1000, max: to / 1000 }, loss: { range: [0, 100] } },
        series: [
          { value: (_u, v) => (v == null ? '—' : fmtShort(v * 1000)) },
          { label: 'Medie (ms)', stroke: line, width: 2, spanGaps: false, value: (_u, v) => (v == null ? '—' : v.toFixed(1)) },
          { label: 'Min (ms)', stroke: line, width: 0, value: (_u, v) => (v == null ? '—' : v.toFixed(1)) },
          { label: 'Max (ms)', stroke: line, width: 0, value: (_u, v) => (v == null ? '—' : v.toFixed(1)) },
          { label: 'Pierderi (%)', stroke: loss, width: 1, scale: 'loss', value: (_u, v) => (v == null ? '—' : Math.round(v) + '%') },
        ],
        bands: [{ series: [3, 2], fill: line + '33' }],
        axes: [
          { stroke: fg, grid: { stroke: grid, width: 1 }, ticks: { stroke: grid }, values: (_u, s) => s.map((v) => fmtShort(v * 1000)) },
          { stroke: fg, grid: { stroke: grid, width: 1 }, ticks: { stroke: grid }, label: 'ms' },
          { side: 1, scale: 'loss', stroke: loss, grid: { show: false }, label: '%' },
        ],
        legend: { show: true },
        cursor: { drag: { x: false, y: false } },
      },
      data,
      host,
    );
  });
</script>

<div class="chart" bind:this={host} role="img" aria-label="Grafic de latență: medie, interval min–max și pierderi">
  {#if points.length === 0}<p class="muted empty">Nu există măsurători în perioada selectată.</p>{/if}
</div>

<style>
  .chart {
    position: relative;
    min-height: 60px;
  }
  .empty {
    margin: 0 0 6px;
  }
  .chart :global(.u-legend) {
    color: var(--fg);
    font-size: 0.85em;
  }
</style>
