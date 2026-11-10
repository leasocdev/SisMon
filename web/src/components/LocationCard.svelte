<script lang="ts">
  import { live } from '../lib/live.svelte';
  import { errorText, fmtLatency, pendingText, severity, STATE_SYMBOL } from '../lib/status';
  import { fmtSince } from '../lib/time';
  import { timelines } from '../lib/timelines.svelte';
  import type { LocationView } from '../lib/types';
  import MiniTimeline from './MiniTimeline.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let { loc, size = 'normal', timeline = true }: { loc: LocationView; size?: 'normal' | 'large'; timeline?: boolean } = $props();

  let pulse = $state(false);
  let prevSeverity: number | null = null;
  let pulseTimer: ReturnType<typeof setTimeout> | null = null;

  $effect(() => {
    const s = severity(loc.state);
    if (prevSeverity !== null && s < prevSeverity) {
      pulse = true;
      if (pulseTimer) clearTimeout(pulseTimer);
      pulseTimer = setTimeout(() => (pulse = false), 3000);
    }
    prevSeverity = s;
  });

  const now = $derived((live.tick, live.serverNow()));
  const tl = $derived(timeline ? timelines.for(loc.id, now) : null);
</script>

<a class="loc-card card state-{loc.state} {size}" class:pulse class:stale={live.stale} href="/locatie/{loc.id}" data-id={loc.id}>
  <div class="head">
    <h3 class="name">{loc.name}</h3>
    <StatusBadge state={loc.state} stale={live.stale} size={size === 'large' ? 'large' : 'normal'} />
  </div>
  <div class="meta">
    {#if loc.since > 0}<span>{fmtSince(loc.since, now)}</span>{/if}
    {#if loc.incomplete}<span class="tag">date incomplete</span>{/if}
  </div>
  <ul class="devices">
    {#each loc.devices as d (d.id)}
      <li class:primary={d.is_primary}>
        <span class="sym t-{d.state}" aria-hidden="true">{STATE_SYMBOL[d.state]}</span>
        <span class="dname">
          {#if d.is_primary}<span class="star" title="Echipament primar">★</span><span class="sr-only">primar</span>{/if}
          {d.name}
        </span>
        <span class="host">{d.host}</span>
        <span class="info">
          {#if d.state === 'error'}<span class="t-error">! Eroare monitor</span>
          {:else if pendingText(d)}<span class="pending">{pendingText(d)}</span>
          {:else if d.state === 'down'}<span class="t-down">{errorText(d.error_code) || 'Indisponibil'}</span>
          {:else}{fmtLatency(d.latency_ms)}{/if}
        </span>
      </li>
    {/each}
  </ul>
  {#if tl}
    <MiniTimeline segments={tl.segments} from={tl.from} to={tl.to} />
  {/if}
</a>

<style>
  .loc-card {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 12px;
    color: var(--fg);
    text-decoration: none;
    border-left: 5px solid var(--c-unknown);
    min-width: 0;
  }
  .loc-card:hover {
    border-color: var(--focus);
  }
  .state-up {
    border-left-color: var(--c-up);
  }
  .state-down {
    border-left-color: var(--c-down);
  }
  .state-degraded {
    border-left-color: var(--c-degraded);
  }
  .state-error {
    border-left-color: var(--c-error);
  }
  .stale {
    border-left-color: var(--c-unknown);
    opacity: 0.85;
  }
  .head {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    align-items: flex-start;
  }
  .name {
    margin: 0;
    font-size: 1.05em;
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    gap: 8px;
    color: var(--muted);
    font-size: 0.86em;
    min-height: 1.2em;
  }
  .tag {
    border: 1px dashed var(--muted);
    border-radius: 3px;
    padding: 0 4px;
  }
  .devices {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.88em;
  }
  .devices li {
    display: grid;
    grid-template-columns: 1.1em minmax(0, 1fr) auto auto;
    gap: 6px;
    align-items: baseline;
  }
  .dname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .star {
    color: var(--c-degraded);
    margin-right: 2px;
  }
  .host {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 0.92em;
  }
  .info {
    text-align: right;
    white-space: nowrap;
  }
  .pending {
    color: var(--c-degraded);
    font-style: italic;
  }
  .large {
    font-size: 1.2em;
  }
  .pulse {
    animation: pulse 0.6s ease-in-out 5;
  }
  @keyframes pulse {
    50% {
      box-shadow: 0 0 0 4px var(--c-down);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .pulse {
      animation: none;
      box-shadow: 0 0 0 3px var(--c-down);
    }
  }
</style>
