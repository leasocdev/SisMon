<script lang="ts">
  // Content of a map marker popup. It is mounted while the popup is open and
  // follows the live state by itself (devices, durations, 24 h timeline).
  import { tick } from 'svelte';
  import { live } from '../lib/live.svelte';
  import { STATE_LABEL, stateText } from '../lib/status';
  import { fmtShort, fmtSince, fmtTime } from '../lib/time';
  import { timelines } from '../lib/timelines.svelte';
  import MiniTimeline from './MiniTimeline.svelte';

  let { id, onresize }: { id: number; onresize?: () => void } = $props();

  const loc = $derived(live.location(id));
  const now = $derived((live.tick, live.serverNow()));
  const bad = $derived(loc ? loc.devices.filter((d) => d.state !== 'up') : []);
  const tl = $derived(timelines.for(id, now));

  /** '14:02' today, '12.11 14:02' on another day. */
  function at(ms: number): string {
    return fmtShort(ms).slice(0, 5) === fmtShort(now).slice(0, 5) ? fmtTime(ms) : fmtShort(ms);
  }

  // keeps the 24 h timelines loaded while the popup is open
  $effect(() => timelines.acquire());

  // Leaflet sizes the popup when it opens: let it re-measure when the content grows or shrinks
  $effect(() => {
    void [loc?.state, bad.length, loc?.devices.length];
    tick().then(() => onresize?.());
  });
</script>

<div class="nw-popup">
  {#if !loc}
    <p>Locația nu mai există.</p>
  {:else}
    <strong class="pname">{loc.name}</strong>
    <div class="pstate t-{loc.state}">
      {stateText(loc.state)}{#if loc.since}{' '}<span class="since">{fmtSince(loc.since, now)} (de la {at(loc.since)})</span>{/if}
    </div>
    {#if bad.length}
      <ul>
        {#each bad as d (d.id)}<li>{d.name} ({d.host}): {STATE_LABEL[d.state]}</li>{/each}
      </ul>
    {/if}
    <div class="ptl">
      <MiniTimeline segments={tl.segments} from={tl.from} to={tl.to} label="Cronologia ultimelor 24 de ore pentru {loc.name}" />
      <div class="axis muted" aria-hidden="true"><span>−24 h</span><span>acum</span></div>
    </div>
    <a href="/locatie/{loc.id}">Detalii</a>
  {/if}
</div>

<style>
  .nw-popup {
    min-width: 240px;
    max-width: 320px;
  }
  .pname {
    font-size: 1.05em;
  }
  .since {
    color: var(--fg);
    font-weight: 400;
  }
  ul {
    margin: 4px 0;
    padding-left: 18px;
  }
  .ptl {
    margin: 8px 0 6px;
  }
  .axis {
    display: flex;
    justify-content: space-between;
    font-size: 0.75em;
    margin-top: 2px;
  }
</style>
