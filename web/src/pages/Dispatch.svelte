<script lang="ts">
  import { onMount } from 'svelte';
  import ConnectionBadge from '../components/ConnectionBadge.svelte';
  import LoadFailed from '../components/LoadFailed.svelte';
  import DemoBanner from '../components/DemoBanner.svelte';
  import { computeGrid } from '../lib/grid';
  import { live } from '../lib/live.svelte';
  import { router } from '../lib/router.svelte';
  import { errorText, sortLocations, STATE_LABEL, STATE_SYMBOL } from '../lib/status';
  import { fmtDate, fmtSince, fmtTime } from '../lib/time';
  import type { LocationView } from '../lib/types';

  // ?rotatie=N in the URL overrides the administrator's setting.
  const queryRot = Number(router.route.query.rotatie);
  const rotateS = $derived(Math.max(10, queryRot > 0 ? queryRot : live.rotationS));
  const rotate = $derived(queryRot > 0 || live.rotationS > 0);
  let showMap = $state(false);

  let gridEl: HTMLDivElement;
  let size = $state({ w: 1600, h: 800 });
  let root: HTMLDivElement;

  const now = $derived((live.tick, live.serverNow()));
  const locs = $derived(sortLocations(live.locations, 'severity'));
  const grid = $derived(computeGrid(locs.length, size.w, size.h));

  /** The device explaining a location's problem, for the band. */
  function culprit(l: LocationView) {
    const primaryDown = l.devices.find((d) => d.is_primary && d.state === 'down');
    if (primaryDown) return primaryDown;
    return l.devices.find((d) => d.state === 'down') ?? l.devices.find((d) => d.state !== 'up');
  }

  function fullscreen() {
    if (document.fullscreenElement) document.exitFullscreen();
    else root.requestFullscreen?.();
  }

  onMount(() => {
    const ro = new ResizeObserver(() => gridEl && (size = { w: gridEl.clientWidth, h: gridEl.clientHeight }));
    ro.observe(gridEl);
    return () => ro.disconnect();
  });
  $effect(() => {
    if (!rotate) {
      showMap = false;
      return;
    }
    const rot = setInterval(() => (showMap = !showMap), rotateS * 1000);
    return () => clearInterval(rot);
  });
</script>

<div class="dispatch" class:stale={live.stale} bind:this={root}>
  <DemoBanner />
  <h1 class="sr-only">Dispecerat</h1>
  <header class="top">
    <div class="ident">
      {#if live.identity.logo_version > 0}<img class="dlogo" src="/branding/logo?v={live.identity.logo_version}" alt="" />{/if}
      <span class="texts">
        <span class="name">{live.identity.name}</span>
        {#if live.identity.subtitle}<span class="sub">{live.identity.subtitle}</span>{/if}
      </span>
    </div>
    <div class="right">
      <ConnectionBadge large />
      <div class="when">
        <span class="clock" aria-label="Ora curentă (Europe/Chisinau)">{fmtTime(now, true)}</span>
        <span class="date">{fmtDate(now)}</span>
      </div>
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
    align-items: center;
    gap: 10px;
    line-height: 1.1;
  }
  .texts {
    display: flex;
    flex-direction: column;
  }
  .name {
    font-weight: 700;
    font-size: 1.3em;
  }
  .dlogo {
    height: 40px;
    max-width: 140px;
    object-fit: contain;
    object-position: left center;
  }
  .sub {
    font-size: 0.72em;
    letter-spacing: 0.08em;
    color: var(--muted);
  }
  .when {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    line-height: 1.1;
  }
  .date {
    color: var(--muted);
  }
  .right {
    margin-left: auto;
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
