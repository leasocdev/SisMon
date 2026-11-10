<script lang="ts">
  import { onMount } from 'svelte';
  import type * as Leaflet from 'leaflet';
  import contours from '../assets/contours.json';
  import { loadLeaflet } from '../lib/leaflet';
  import { live } from '../lib/live.svelte';
  import { severity, STATE_LABEL, STATE_SYMBOL, stateText } from '../lib/status';
  import { TileHealth } from '../lib/tilehealth';
  import type { LocationView, State } from '../lib/types';

  let { fill = true }: { fill?: boolean } = $props();

  let host: HTMLDivElement;
  let fallback = $state(false);
  let ready = $state(false);

  let L: typeof Leaflet;
  let map: Leaflet.Map;
  let tiles: Leaflet.TileLayer;
  let outline: Leaflet.GeoJSON | null = null;
  let cluster: Leaflet.MarkerClusterGroup;
  const markers = new Map<number, { m: Leaflet.Marker; state: State; name: string }>();
  const health = new TileHealth();
  let fitted = false;

  const located = $derived(live.locations.filter((l) => l.lat != null && l.lon != null));
  const unlocated = $derived(live.locations.filter((l) => l.lat == null || l.lon == null));

  function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls = '', text = ''): HTMLElementTagNameMap[K] {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }

  function icon(l: LocationView): Leaflet.DivIcon {
    const s = el('span', `nw-marker state-${l.state}`, STATE_SYMBOL[l.state]);
    s.dataset.id = String(l.id);
    s.setAttribute('aria-hidden', 'true');
    return L.divIcon({ html: s, className: 'nw-icon', iconSize: [26, 26], iconAnchor: [13, 13], popupAnchor: [0, -12] });
  }

  function popup(id: number): HTMLElement {
    const l = live.location(id);
    const box = el('div', 'nw-popup');
    if (!l) {
      box.append(el('p', '', 'Locația nu mai există.'));
      return box;
    }
    box.append(el('strong', 'pname', l.name));
    box.append(el('div', `pstate t-${l.state}`, stateText(l.state)));
    const bad = l.devices.filter((d) => d.state !== 'up');
    if (bad.length) {
      const ul = el('ul');
      for (const d of bad) ul.append(el('li', '', `${d.name} (${d.host}): ${STATE_LABEL[d.state]}`));
      box.append(ul);
    }
    const a = el('a', '', 'Detalii');
    a.href = `/locatie/${l.id}`;
    box.append(a);
    return box;
  }

  function clusterIcon(c: Leaflet.MarkerCluster): Leaflet.DivIcon {
    let worst: State = 'up';
    for (const m of c.getAllChildMarkers()) {
      const s = (m.options as { nwState?: State }).nwState ?? 'unknown';
      if (severity(s) < severity(worst)) worst = s;
    }
    const s = el('span', `nw-cluster state-${worst}`, String(c.getChildCount()));
    return L.divIcon({ html: s, className: 'nw-icon', iconSize: [34, 34] });
  }

  function sync(list: LocationView[]) {
    if (!ready) return;
    const seen = new Set<number>();
    for (const l of list) {
      seen.add(l.id);
      const cur = markers.get(l.id);
      const pos = L.latLng(l.lat!, l.lon!);
      if (!cur) {
        const m = L.marker(pos, { icon: icon(l), title: l.name, keyboard: true, nwState: l.state } as Leaflet.MarkerOptions);
        m.bindPopup(() => popup(l.id));
        cluster.addLayer(m);
        markers.set(l.id, { m, state: l.state, name: l.name });
        continue;
      }
      if (!cur.m.getLatLng().equals(pos)) cur.m.setLatLng(pos);
      if (cur.state !== l.state || cur.name !== l.name) {
        (cur.m.options as { nwState?: State }).nwState = l.state;
        cur.m.setIcon(icon(l));
        cur.state = l.state;
        cur.name = l.name;
        if (cur.m.isPopupOpen()) cur.m.setPopupContent(popup(l.id));
      }
    }
    for (const [id, cur] of markers) {
      if (!seen.has(id)) {
        cluster.removeLayer(cur.m);
        markers.delete(id);
      }
    }
    cluster.refreshClusters();
    if (!fitted && list.length && host.clientWidth > 0) {
      fitted = true;
      map.fitBounds(L.latLngBounds(list.map((l) => [l.lat!, l.lon!] as [number, number])), { padding: [40, 40], maxZoom: 12, animate: false });
    }
  }

  function enterFallback() {
    if (fallback) return;
    fallback = true;
    map.removeLayer(tiles);
    outline = L.geoJSON(contours as GeoJSON.FeatureCollection, {
      style: { className: 'contour', weight: 1, color: '#888', fillColor: '#888', fillOpacity: 0.08 },
    }).addTo(map);
    outline.bringToBack();
  }

  function exitFallback() {
    if (!fallback) return;
    fallback = false;
    outline?.remove();
    outline = null;
    health.reset();
    tiles.addTo(map);
    tiles.redraw();
  }

  function probe() {
    if (!fallback) return;
    const img = new Image();
    img.onload = exitFallback;
    img.src = `/tiles/2/2/1.png?probe=${Date.now()}`;
  }

  onMount(() => {
    let alive = true;
    let timer: ReturnType<typeof setInterval> | null = null;
    loadLeaflet().then((lib) => {
      if (!alive) return;
      L = lib;
      map = L.map(host, { minZoom: 2, maxZoom: 18, worldCopyJump: true }).setView([47.0, 28.5], 7);
      map.attributionControl.setPrefix('<a href="https://leafletjs.com">Leaflet</a>');
      tiles = L.tileLayer('/tiles/{z}/{x}/{y}.png', {
        minZoom: 2,
        maxZoom: 18,
        attribution: '© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
      });
      tiles.on('tileload', () => health.record(true, Date.now()));
      tiles.on('tileerror', (e: Leaflet.TileErrorEvent) => {
        const img = e.tile as HTMLImageElement;
        if (!img.dataset.retry) {
          img.dataset.retry = '1';
          const src = img.src;
          setTimeout(() => (img.src = src.split('?')[0] + '?retry=1'), 2000);
          return;
        }
        health.record(false, Date.now());
        if (health.shouldFallback(Date.now())) enterFallback();
      });
      tiles.addTo(map);
      cluster = L.markerClusterGroup({ iconCreateFunction: clusterIcon, maxClusterRadius: 40, showCoverageOnHover: false });
      map.addLayer(cluster);
      ready = true;
      sync(located);
      timer = setInterval(probe, 60_000);
    });
    const ro = new ResizeObserver(() => {
      if (!ready) return;
      map.invalidateSize();
      if (!fitted) sync(located);
    });
    ro.observe(host);
    return () => {
      alive = false;
      ro.disconnect();
      if (timer) clearInterval(timer);
      map?.stop();
      map?.off();
      map?.remove();
    };
  });

  $effect(() => {
    sync(located);
  });
</script>

<div class="mapwrap" class:fill>
  <div class="map" bind:this={host} aria-label="Hartă locații"></div>
  {#if fallback}
    <p class="fallback-note" role="status">Dalele OpenStreetMap nu se încarcă — se afișează contururi locale. Se reîncearcă automat.</p>
  {/if}
</div>
{#if unlocated.length}
  <p class="no-coords muted">Fără coordonate: {unlocated.map((l) => l.name).join(', ')}</p>
{/if}

<style>
  .mapwrap {
    position: relative;
    height: 420px;
  }
  .mapwrap.fill {
    height: 100%;
  }
  .map {
    position: absolute;
    inset: 0;
    background: var(--surface-2);
  }
  .fallback-note {
    position: absolute;
    left: 50%;
    top: 8px;
    transform: translateX(-50%);
    z-index: 500;
    margin: 0;
    padding: 4px 10px;
    border-radius: 4px;
    background: var(--warn-bg);
    border: 1px solid var(--c-degraded);
    color: var(--fg);
    font-size: 0.9em;
  }
  .no-coords {
    margin: 6px 0 0;
    font-size: 0.9em;
  }
  :global(.nw-icon) {
    background: none;
    border: none;
  }
  :global(.nw-marker),
  :global(.nw-cluster) {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    font-weight: 800;
    border: 2px solid var(--surface);
    box-shadow: 0 0 0 1px rgb(0 0 0 / 0.35);
    cursor: pointer;
  }
  :global(.nw-marker.state-up),
  :global(.nw-cluster.state-up) {
    background: var(--c-up);
    color: var(--on-up);
  }
  :global(.nw-marker.state-down),
  :global(.nw-cluster.state-down) {
    background: var(--c-down);
    color: var(--on-down);
  }
  :global(.nw-marker.state-degraded),
  :global(.nw-cluster.state-degraded) {
    background: var(--c-degraded);
    color: var(--on-degraded);
  }
  :global(.nw-marker.state-unknown),
  :global(.nw-cluster.state-unknown) {
    background: var(--c-unknown);
    color: var(--on-unknown);
  }
  :global(.nw-marker.state-error),
  :global(.nw-cluster.state-error) {
    background: var(--c-error);
    color: var(--on-error);
  }
  :global(.nw-popup ul) {
    margin: 4px 0;
    padding-left: 18px;
  }
  :global(.nw-popup .pname) {
    font-size: 1.05em;
  }
  :global(.leaflet-popup-content-wrapper),
  :global(.leaflet-popup-tip) {
    background: var(--surface);
    color: var(--fg);
  }
  :global(.leaflet-container) {
    font: inherit;
    background: var(--surface-2);
  }
  :global(.leaflet-container a) {
    color: var(--link);
  }
</style>
