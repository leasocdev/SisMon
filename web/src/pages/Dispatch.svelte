<script lang="ts">
  import { onMount, tick } from 'svelte';
  import ConnectionBadge from '../components/ConnectionBadge.svelte';
  import LoadFailed from '../components/LoadFailed.svelte';
  import DemoBanner from '../components/DemoBanner.svelte';
  import { computeGrid } from '../lib/grid';
  import { live } from '../lib/live.svelte';
  import { router } from '../lib/router.svelte';
  import { errorText, severity, sortLocations, STATE_LABEL, STATE_SYMBOL } from '../lib/status';
  import { fmtDate, fmtSince, fmtTime } from '../lib/time';
  import type { LocationView } from '../lib/types';

  const rotateS = Math.max(10, Number(router.route.query.rotatie) || 0);
  const rotate = Number(router.route.query.rotatie) > 0;
  let showMap = $state(false);

  let gridEl: HTMLDivElement;
  let size = $state({ w: 1600, h: 800 });
  let root: HTMLDivElement;

  const now = $derived((live.tick, live.serverNow()));
  const locs = $derived(sortLocations(live.locations, 'severity'));
  const problems = $derived(locs.filter((l) => severity(l.state) < 2));
  const grid = $derived(computeGrid(locs.length, size.w, size.h));

  // Problem band: show what fits on one line and a "+N" counter for the rest.
  let itemsEl = $state<HTMLDivElement>();
  let visible = $state(Infinity);
  let moreEl = $state<HTMLSpanElement>();
  function measure() {
    if (!itemsEl) {
      visible = Infinity;
      return;
    }
    const kids = [...itemsEl.children] as HTMLElement[];
    const fits = (limit: number) => {
      let n = 0;
      for (const k of kids) {
        if (k.offsetLeft + k.offsetWidth > limit) break;
        n++;
      }
      return n;
    };
    // width available if the "+N" badge were not shown, and the badge width
    const badge = moreEl ? moreEl.offsetWidth + 6 : 0;
    const full = itemsEl.clientWidth + badge;
    if (fits(full) >= kids.length) {
      visible = Infinity;
      return;
    }
    visible = fits(full - Math.max(badge, 64));
  }
  $effect(() => {
    void problems.length;
    void size.w;
    tick().then(measure);
  });

  /** The device explaining a location's problem, for the band. */
  function culprit(l: LocationView) {
    const primaryDown = l.devices.find((d) => d.is_primary && d.state === 'down');
    if (primaryDown) return primaryDown;
    return l.devices.find((d) => d.state === 'down') ?? l.devices.find((d) => d.state !== 'up');
  }

  function problemSince(l: LocationView): number {
    return l.since || culprit(l)?.since || now;
  }

  function fullscreen() {
    if (document.fullscreenElement) document.exitFullscreen();
    else root.requestFullscreen?.();
  }

  onMount(() => {
    const ro = new ResizeObserver(() => gridEl && (size = { w: gridEl.clientWidth, h: gridEl.clientHeight }));
    ro.observe(gridEl);
    const rot = rotate ? setInterval(() => (showMap = !showMap), rotateS * 1000) : null;
    return () => {
      ro.disconnect();
      if (rot) clearInterval(rot);
    };
  });
</script>

<div class="dispatch" class:stale={live.stale} bind:this={root}>
  <DemoBanner />
  <h1 class="sr-only">Dispecerat</h1>
  <header class="top">
    <div class="ident">
      {#if live.identity.logo_version > 0}<img class="dlogo" src="/branding/logo?v={live.identity.logo_version}" alt="" />{/if}
      <span class="name">{live.identity.name}</span>
      <span class="date">{fmtDate(now)}</span>
    </div>
    <div class="band" aria-label="Locații cu probleme">
      {#if problems.length === 0}
        <span class="ok">✓ Toate locațiile funcționează</span>
      {:else}
        <div class="items" bind:this={itemsEl}>
          {#each problems as l, i (l.id)}
            <span class="prob f-{l.state}" class:cut={i >= visible}>
              {l.name} · {culprit(l)?.name ?? STATE_LABEL[l.state]} · {fmtSince(problemSince(l), now)}
            </span>
          {/each}
        </div>
        {#if visible < problems.length}
          <span class="more" bind:this={moreEl} title="Încă {problems.length - visible} locații cu probleme">+{problems.length - visible}</span>
        {/if}
      {/if}
    </div>
    <div class="right">
      <ConnectionBadge large />
      <span class="clock" aria-label="Ora curentă (Europe/Chisinau)">{fmtTime(now, true)}</span>
      <button type="button" class="fs" onclick={fullscreen} aria-label="Ecran complet">⛶</button>
    </div>
  </header>
  {#if live.stale}
    <div class="stale-banner" role="alert">DATE NEACTUALIZATE</div>
  {/if}
  {#if rotate}
    <div class="mapbox" class:hidden={!showMap}>
      {#await import('../components/MapView.svelte') then m}<m.default />{:catch}<LoadFailed />{/await}
    </div>
  {/if}
  <div class="grid" class:hidden={showMap} bind:this={gridEl} style:--cols={grid.cols} style:--rows={grid.rows} style:--font="{grid.fontPx}px">
    {#each locs as l (l.id)}
      {@const c = culprit(l)}
      <a class="tile f-{live.stale ? 'unknown' : l.state}" href="/locatie/{l.id}">
        <span class="tname">{l.name}</span>
        <span class="tstate"><span aria-hidden="true">{STATE_SYMBOL[l.state]}</span> {STATE_LABEL[l.state]}{#if live.stale} · neactualizat{/if}</span>
        {#if l.state !== 'up' && c}
          <span class="tdet">{c.name}{c.error_code ? ' · ' + errorText(c.error_code) : ''}</span>
        {/if}
        {#if l.since > 0 && l.state !== 'up'}<span class="tsince">{fmtSince(l.since, now)}</span>{/if}
      </a>
    {/each}
  </div>
</div>

<style>
  .dispatch {
    position: fixed;
    inset: 0;
    display: flex;
    flex-direction: column;
    background: var(--bg);
    border: 6px solid transparent;
  }
  .dispatch.stale {
    border-color: var(--c-unknown);
  }
  .top {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 6px 12px;
    border-bottom: 1px solid var(--border);
    min-height: 64px;
  }
  .ident {
    display: flex;
    flex-direction: column;
    line-height: 1.1;
  }
  .name {
    font-weight: 700;
    font-size: 1.3em;
  }
  .dlogo {
    height: 40px;
    max-width: 140px;
    object-fit: contain;
  }
  .date {
    color: var(--muted);
  }
  .band {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 1.1em;
  }
  .items {
    position: relative;
    flex: 1;
    min-width: 0;
    display: flex;
    gap: 6px;
    overflow: hidden;
    white-space: nowrap;
  }
  .cut {
    visibility: hidden;
  }
  .more {
    flex-shrink: 0;
    font-weight: 800;
    padding: 3px 10px;
    border-radius: 4px;
    background: var(--c-down);
    color: var(--on-down);
  }
  .prob {
    padding: 3px 10px;
    border-radius: 4px;
    font-weight: 600;
  }
  .ok {
    color: var(--c-up);
    font-weight: 600;
  }
  .right {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .clock {
    font-size: 2.2em;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .fs {
    opacity: 0.4;
    font-size: 1.3em;
  }
  .fs:hover,
  .fs:focus-visible {
    opacity: 1;
  }
  .stale-banner {
    text-align: center;
    font-size: 2em;
    font-weight: 800;
    letter-spacing: 0.1em;
    background: var(--c-unknown);
    color: var(--on-unknown);
    padding: 4px;
  }
  .grid {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: repeat(var(--cols), minmax(0, 1fr));
    grid-template-rows: repeat(var(--rows), minmax(0, 1fr));
    gap: 6px;
    padding: 6px;
    font-size: var(--font);
    overflow: hidden;
  }
  .hidden {
    display: none;
  }
  .mapbox {
    flex: 1;
    min-height: 0;
    padding: 6px;
  }
  .tile {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 0.15em;
    padding: 0.3em 0.5em;
    border-radius: 6px;
    text-decoration: none;
    overflow: hidden;
    min-height: 0;
  }
  .tile.f-up {
    background: var(--surface);
    color: var(--fg);
    border-left: 0.35em solid var(--c-up);
  }
  .tname {
    font-weight: 700;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tstate {
    font-size: 0.8em;
    font-weight: 600;
  }
  .tile.f-up .tstate {
    color: var(--c-up);
  }
  .tdet,
  .tsince {
    font-size: 0.7em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
