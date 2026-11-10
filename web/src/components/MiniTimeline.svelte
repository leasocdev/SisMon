<script lang="ts">
  import { STATE_LABEL } from '../lib/status';
  import { fmtShort, fmtTime } from '../lib/time';
  import type { Segment } from '../lib/types';

  let { segments, from, to, label = 'Cronologie 24 h', tall = false }: { segments: Segment[]; from: number; to: number; label?: string; tall?: boolean } =
    $props();

  const span = $derived(Math.max(1, to - from));
  const parts = $derived(
    segments
      .filter((s) => s.end > from && s.start < to)
      .map((s) => {
        const a = Math.max(s.start, from);
        const b = Math.min(s.end, to);
        return { ...s, left: ((a - from) / span) * 100, width: Math.max(((b - a) / span) * 100, 0.2), a, b };
      }),
  );
</script>

<div class="mini-timeline" class:tall role="img" aria-label={label}>
  {#each parts as p (p.start)}
    <div class="seg f-{p.state}" style:left="{p.left}%" style:width="{p.width}%" title="{STATE_LABEL[p.state]} · {fmtShort(p.a)}–{fmtTime(p.b)}"></div>
  {/each}
</div>

<style>
  .mini-timeline {
    position: relative;
    height: 12px;
    border-radius: 3px;
    background: repeating-linear-gradient(135deg, var(--track) 0 4px, transparent 4px 8px), var(--surface);
    border: 1px solid var(--muted);
    overflow: hidden;
  }
  .tall {
    height: 22px;
  }
  .seg.f-unknown {
    background: repeating-linear-gradient(135deg, var(--c-unknown) 0 3px, var(--surface) 3px 6px);
  }
  .seg {
    box-shadow: inset 0 0 0 1px rgb(0 0 0 / 0.25);
  }
  .seg {
    position: absolute;
    top: 0;
    bottom: 0;
  }
</style>
