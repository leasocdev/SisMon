<script lang="ts">
  import { fmtPct, STATE_LABEL } from '../lib/status';
  import type { Durations, State } from '../lib/types';

  let { durations }: { durations: Durations } = $props();

  const ORDER: State[] = ['up', 'degraded', 'down', 'error', 'unknown'];
  const total = $derived(ORDER.reduce((s, k) => s + durations[k], 0));
  const parts = $derived(total > 0 ? ORDER.filter((k) => durations[k] > 0).map((k) => ({ k, pct: (100 * durations[k]) / total })) : []);
  const label = $derived(parts.map((p) => `${STATE_LABEL[p.k]} ${fmtPct(p.pct)}`).join(', ') || 'Fără date');
</script>

<div class="bar" role="img" aria-label={label} title={label}>
  {#each parts as p (p.k)}
    <span class="part f-{p.k}" style:width="{p.pct}%"></span>
  {/each}
</div>

<style>
  .bar {
    display: flex;
    height: 14px;
    border-radius: 3px;
    overflow: hidden;
    background: var(--surface-2);
    min-width: 160px;
  }
  .part {
    display: block;
    height: 100%;
  }
</style>
