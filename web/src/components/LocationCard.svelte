<script lang="ts">
  import { live } from '../lib/live.svelte';
  import { cardDevices, errorText, fmtLatency, pendingText, severity, STATE_SYMBOL } from '../lib/status';
  import { timelines } from '../lib/timelines.svelte';
  import type { DeviceView, LocationView } from '../lib/types';
  import MiniTimeline from './MiniTimeline.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let {
    loc,
    size = 'normal',
    timeline = true,
    compact = false,
  }: { loc: LocationView; size?: 'normal' | 'large'; timeline?: boolean; compact?: boolean } = $props();

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
  const devs = $derived(cardDevices(loc.state, loc.devices, compact));

  /** Status message shown on its own line under the device name (empty for a healthy device). */
  function message(d: DeviceView): { text: string; cls: string } | null {
    if (d.state === 'error') return { text: '! Eroare monitor', cls: 't-error' };
    const p = pendingText(d);
    if (p) return { text: p, cls: 'pending' };
    if (d.state === 'down') return { text: errorText(d.error_code) || 'Indisponibil', cls: 't-down' };
    return null;
  }
</script>

<a class="loc-card card state-{loc.state} {size}" class:pulse class:stale={live.stale} href="/locatie/{loc.id}" data-id={loc.id}>
  <div class="head">
    <h3 class="name">{loc.name}</h3>
    <StatusBadge state={loc.state} stale={live.stale} size={size === 'large' ? 'large' : 'normal'} />
  </div>
  {#if loc.incomplete}
    <div class="meta"><span class="tag">date incomplete</span></div>
  {/if}
  <ul class="devices">
    {#each devs.shown as d (d.id)}
      {@const msg = message(d)}
      <li class:primary={d.is_primary} class:bad={d.state !== 'up'} class="d-{d.state}">
        <span class="dev-sym t-{d.state}" aria-hidden="true">{STATE_SYMBOL[d.state]}</span>
        <span class="dname">
          {#if d.is_primary}<span class="star" title="Echipament primar">★</span><span class="sr-only">primar</span>{/if}
          {d.name}
        </span>
        <span class="host" class:sep={d.latency_ms != null}>{d.host}</span>
        <span class="info">{d.latency_ms != null ? fmtLatency(d.latency_ms) : ''}</span>
        {#if msg}<span class="msg {msg.cls}">{msg.text}</span>{/if}
      </li>
    {/each}
  </ul>
  {#if loc.devices.length === 0}
    <p class="more">Toate echipamentele sunt dezactivate temporar.</p>
  {/if}
  {#if devs.hidden > 0}
    <p class="more">+ {devs.hidden} {devs.hidden === 1 ? 'echipament funcțional' : 'echipamente funcționale'}</p>
  {/if}
  {#if tl}
    <div class="tl">
      <MiniTimeline segments={tl.segments} from={tl.from} to={tl.to} />
    </div>
  {/if}
</a>

<style>
  .loc-card {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 16px 18px;
    font-size: 1.08em;
    color: var(--fg);
    text-decoration: none;
    border: 2px solid var(--sc, var(--c-unknown));
    border-left-width: 9px;
    background: color-mix(in srgb, var(--sc, var(--c-unknown)) 14%, var(--surface));
    min-width: 0;
  }
  .loc-card:hover {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  .state-up {
    --sc: var(--c-up);
  }
  .state-down {
    --sc: var(--c-down);
    box-shadow: 0 0 0 1px var(--c-down);
  }
  .state-degraded {
    --sc: var(--c-degraded);
  }
  .state-error {
    --sc: var(--c-error);
  }
  .stale {
    --sc: var(--c-unknown);
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
    font-size: 1.12em;
    font-weight: 700;
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    gap: 8px;
    font-size: 0.86em;
  }
  .tag {
    border: 1px dashed var(--muted);
    border-radius: 3px;
    padding: 0 4px;
  }
  /* Un singur grid pentru toată lista: coloanele (nume, IP, latență) se aliniază între rânduri. */
  .devices {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: 1.3em minmax(0, 1fr) auto auto;
    column-gap: 8px;
    row-gap: 3px;
    font-size: 0.9em;
  }
  .devices li {
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: subgrid;
    row-gap: 1px;
    align-items: center;
    padding: 2px 0;
  }
  /* Rândurile cu probleme ies în evidență în interiorul cardului. */
  .devices li.bad {
    margin: 0 -6px;
    padding: 3px 6px;
    border-radius: 4px;
    background: color-mix(in srgb, var(--dc, var(--c-unknown)) 22%, var(--surface));
  }
  .d-down {
    --dc: var(--c-down);
  }
  .d-degraded {
    --dc: var(--c-degraded);
  }
  .d-error {
    --dc: var(--c-error);
  }
  .dname {
    overflow-wrap: anywhere;
  }
  .star {
    color: #f2b400;
    text-shadow: 0 0 1px rgb(0 0 0 / 0.6);
    margin-right: 2px;
  }
  .host {
    padding-right: 10px;
    border-right: 2px solid transparent;
    color: var(--fg);
    opacity: 0.75;
    font-family: var(--mono);
    font-size: 0.92em;
    white-space: nowrap;
  }
  .host.sep {
    border-right-color: color-mix(in srgb, var(--fg) 45%, transparent);
  }
  .info {
    min-width: 4.5em;
    font-weight: 600;
    text-align: right;
    white-space: nowrap;
  }
  /* Mesajul de stare pe rândul al doilea, sub nume — nu mai împinge numele afară. */
  .msg {
    grid-column: 2 / -1;
    font-weight: 600;
    font-size: 0.92em;
  }
  .pending {
    color: var(--c-degraded-text);
    font-style: italic;
  }
  /* Echipament funcțional: bifă simplă, fără cerc. */
  .dev-sym.t-up {
    border: none;
    background: none;
  }
  .more {
    margin: -4px 0 0;
    font-size: 0.82em;
    opacity: 0.75;
  }
  /* Bara cronologică lipită de baza cardului, la aceeași înălțime pe tot rândul. */
  .tl {
    margin-top: auto;
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
