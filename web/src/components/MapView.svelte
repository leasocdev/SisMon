<script lang="ts">
  import { mount, onMount, unmount } from 'svelte';
  import type * as Leaflet from 'leaflet';
  import contours from '../assets/contours.json';
  import { loadLeaflet } from '../lib/leaflet';
  import { live } from '../lib/live.svelte';
  import { severity, STATE_SYMBOL, stateText } from '../lib/status';
  import { fmtDuration } from '../lib/time';
  import MapPopup from './MapPopup.svelte';
  import { retryDelay, TILE_RETRIES, TileHealth } from '../lib/tilehealth';
  import type { LocationView, State } from '../lib/types';

  let { fill = true }: { fill?: boolean } = $props();

  let host: HTMLDivElement;
  let wrap: HTMLDivElement;
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
    s.append(el('span', 'nw-label', l.name));
    return L.divIcon({ html: s, className: 'nw-icon', iconSize: [26, 26], iconAnchor: [13, 13], popupAnchor: [0, -12] });
  }

  // Popup content: a live Svelte component, mounted while the popup is open.
  const popups = new Map<number, { box: HTMLElement; comp: ReturnType<typeof mount> }>();

  function popup(id: number): HTMLElement {
    const open = popups.get(id);
    if (open) return open.box; // Leaflet may ask again while it is open
    const box = el('div');
    const comp = mount(MapPopup, { target: box, props: { id, onresize: () => markers.get(id)?.m.getPopup()?.update() } });
    popups.set(id, { box, comp });
    return box;
  }

  function closePopup(id: number) {
    const open = popups.get(id);
    if (!open) return;
    popups.delete(id);
    unmount(open.comp);
  }

  /** Hover hint: 'Cahul · ✕ Indisponibilă · 35 min'. An element, never a
   * string: Leaflet would insert a string as HTML (location names are text). */
  function hint(id: number): HTMLElement {
    const l = live.location(id);
    if (!l) return el('span');
    const parts = [l.name, stateText(l.state)];
    if (l.since) parts.push(fmtDuration((live.serverNow() - l.since) / 1000));
    return el('span', '', parts.join(' · '));
  }

  // Divs have no alt text: name the marker for screen readers (its title
  // would show a second, native tooltip next to the hover hint).
  function nameMarker(m: Leaflet.Marker, name: string) {
    m.getElement()?.setAttribute('aria-label', name);
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

  // Labels: placed right/left/above/below the marker, worst state first; a label
  // that would overlap another label or marker is hidden until zoomed in.
  let rafId = 0;
  function scheduleLabels() {
    if (!rafId) rafId = requestAnimationFrame(() => ((rafId = 0), placeLabels()));
  }
  type Rect = { x1: number; y1: number; x2: number; y2: number };
  const hit = (a: Rect, b: Rect) => a.x1 < b.x2 && a.x2 > b.x1 && a.y1 < b.y2 && a.y2 > b.y1;

  function placeLabels() {
    if (!ready || !host) return;
    const size = map.getSize();
    const vis: { pt: Leaflet.Point; label: HTMLElement; state: State }[] = [];
    const placed: Rect[] = [];
    for (const cur of markers.values()) {
      if (cluster.getVisibleParent(cur.m) !== cur.m) continue;
      const label = cur.m.getElement()?.querySelector<HTMLElement>('.nw-label');
      if (!label) continue;
      const pt = map.latLngToContainerPoint(cur.m.getLatLng());
      placed.push({ x1: pt.x - 14, y1: pt.y - 14, x2: pt.x + 14, y2: pt.y + 14 });
      vis.push({ pt, label, state: cur.state });
    }
    // cluster bubbles also take room
    host.querySelectorAll<HTMLElement>('.nw-cluster').forEach((c) => {
      const r = c.getBoundingClientRect();
      const o = host.getBoundingClientRect();
      placed.push({ x1: r.left - o.left - 2, y1: r.top - o.top - 2, x2: r.right - o.left + 2, y2: r.bottom - o.top + 2 });
    });
    vis.sort((a, b) => severity(a.state) - severity(b.state));
    for (const v of vis) {
      const w = v.label.offsetWidth;
      const h = v.label.offsetHeight;
      const { x, y } = v.pt;
      const options: [string, Rect][] = [
        ['right', { x1: x + 17, y1: y - h / 2, x2: x + 17 + w, y2: y + h / 2 }],
        ['left', { x1: x - 17 - w, y1: y - h / 2, x2: x - 17, y2: y + h / 2 }],
        ['top', { x1: x - w / 2, y1: y - 17 - h, x2: x + w / 2, y2: y - 17 }],
        ['bottom', { x1: x - w / 2, y1: y + 17, x2: x + w / 2, y2: y + 17 + h }],
      ];
      let chosen = '';
      for (const [pos, r] of options) {
        if (r.x1 < 0 || r.y1 < 0 || r.x2 > size.x || r.y2 > size.y) continue;
        if (placed.some((p) => hit(p, r))) continue;
        chosen = pos;
        placed.push(r);
        break;
      }
      v.label.className = chosen ? `nw-label pos-${chosen}` : 'nw-label';
    }
  }

  function toggleFullscreen() {
    if (document.fullscreenElement) document.exitFullscreen();
    else wrap.requestFullscreen?.();
  }

  /** Fits every located site in the view with a minimal margin. */
  function recenter(animate = true) {
    if (!ready || !located.length) return;
    map.invalidateSize();
    const b = L.latLngBounds(located.map((l) => [l.lat!, l.lon!] as [number, number]));
    map.fitBounds(b, { padding: [12, 12], maxZoom: located.length === 1 ? 14 : 17, animate });
  }

  function sync(list: LocationView[]) {
    if (!ready) return;
    const seen = new Set<number>();
    for (const l of list) {
      seen.add(l.id);
      const cur = markers.get(l.id);
      const pos = L.latLng(l.lat!, l.lon!);
      if (!cur) {
        const m = L.marker(pos, { icon: icon(l), keyboard: true, nwState: l.state } as Leaflet.MarkerOptions);
        m.bindPopup(() => popup(l.id));
        m.bindTooltip(() => hint(l.id), { direction: 'top', offset: [0, -14], className: 'nw-hint' });
        m.on('popupopen', () => m.closeTooltip());
        m.on('popupclose', () => closePopup(l.id));
        m.on('add', () => nameMarker(m, markers.get(l.id)?.name ?? l.name));
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
        nameMarker(cur.m, l.name);
      }
    }
    for (const [id, cur] of markers) {
      if (!seen.has(id)) {
        closePopup(id);
        cluster.removeLayer(cur.m);
        markers.delete(id);
      }
    }
    cluster.refreshClusters();
    scheduleLabels();
    if (!fitted && list.length && host.clientWidth > 0) {
      fitted = true;
      recenter(false);
    }
  }

  // Pending tile retries. They are cancelled when the tile layer goes away:
  // a late retry of a removed tile would report into the fresh TileHealth
  // after recovery and switch straight back to the contours.
  const retries = new Set<ReturnType<typeof setTimeout>>();

  function cancelRetries() {
    for (const t of retries) clearTimeout(t);
    retries.clear();
  }

  function enterFallback() {
    if (fallback) return;
    fallback = true;
    cancelRetries();
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
    // addTo rebuilds every tile at the rounded zoom. No redraw(): in Leaflet
    // 1.9 it skips the rounding, so a fractional zoom (zoomSnap 0.1) asked
    // for tiles like /tiles/8.7/… that always fail and re-entered the fallback.
    tiles.addTo(map);
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
      map = L.map(host, { minZoom: 2, maxZoom: 18, worldCopyJump: true, zoomControl: false, zoomSnap: 0.1, zoomDelta: 1 }).setView([47.0, 28.5], 7);
      map.attributionControl.setPrefix('<a href="https://leafletjs.com">Leaflet</a>');
      tiles = L.tileLayer('/tiles/{z}/{x}/{y}.png', {
        minZoom: 2,
        maxZoom: 18,
        attribution: '© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
      });
      tiles.on('tileload', (e: Leaflet.TileEvent) => {
        if (!fallback && e.tile.isConnected) health.record(true, Date.now());
      });
      tiles.on('tileerror', (e: Leaflet.TileErrorEvent) => {
        const img = e.tile as HTMLImageElement;
        if (fallback || !img.isConnected) return; // a tile of a layer that is gone
        const attempt = Number(img.dataset.retry ?? 0) + 1;
        if (attempt <= TILE_RETRIES) {
          img.dataset.retry = String(attempt);
          const src = img.src.split('?')[0];
          const t = setTimeout(() => {
            retries.delete(t);
            if (img.isConnected) img.src = `${src}?retry=${attempt}`;
          }, retryDelay(attempt, Math.random()));
          retries.add(t);
          return;
        }
        health.record(false, Date.now());
        if (health.shouldFallback(Date.now())) enterFallback();
      });
      tiles.addTo(map);
      const Tools = L.Control.extend({
        onAdd() {
          const bar = el('div', 'leaflet-bar nw-tools');
          const btn = (label: string, title: string, fn: () => void) => {
            const b = el('a', '', label);
            b.href = '#';
            b.role = 'button';
            b.title = title;
            b.setAttribute('aria-label', title);
            b.addEventListener('click', (e) => {
              e.preventDefault();
              if (!b.classList.contains('leaflet-disabled')) fn();
            });
            bar.append(b);
            return b;
          };
          const zin = btn('+', 'Mărește', () => map.zoomIn());
          const zout = btn('−', 'Micșorează', () => map.zoomOut());
          btn('⌖', 'Centrează: încadrează toate locațiile în fereastră', () => recenter());
          const full = btn('⛶', 'Ecran complet (Esc pentru ieșire)', toggleFullscreen);
          const limits = () => {
            zin.classList.toggle('leaflet-disabled', map.getZoom() >= map.getMaxZoom());
            zout.classList.toggle('leaflet-disabled', map.getZoom() <= map.getMinZoom());
          };
          map.on('zoomend', limits);
          limits();
          document.addEventListener('fullscreenchange', () => {
            const on = !!document.fullscreenElement;
            const t = on ? 'Ieșire din ecran complet' : 'Ecran complet (Esc pentru ieșire)';
            full.title = t;
            full.setAttribute('aria-label', t);
            // the box just changed size: refit so the sites sit close to the new edges
            setTimeout(() => recenter(false), 100);
          });
          L.DomEvent.disableClickPropagation(bar);
          return bar;
        },
      });
      new Tools({ position: 'topright' }).addTo(map);
      map.on('moveend zoomend resize', scheduleLabels);
      cluster = L.markerClusterGroup({ iconCreateFunction: clusterIcon, maxClusterRadius: 40, showCoverageOnHover: false });
      map.addLayer(cluster);
      cluster.on('animationend', scheduleLabels);
      ready = true;
      sync(located);
      timer = setInterval(probe, 60_000);
    });
    const ro = new ResizeObserver(() => {
      if (!ready) return;
      map.invalidateSize();
      if (!fitted) sync(located);
      scheduleLabels();
    });
    ro.observe(host);
    return () => {
      alive = false;
      ready = false;
      if (rafId) cancelAnimationFrame(rafId);
      ro.disconnect();
      if (timer) clearInterval(timer);
      cancelRetries();
      for (const id of [...popups.keys()]) closePopup(id);
      map?.stop();
      map?.off();
      map?.remove();
    };
  });

  $effect(() => {
    sync(located);
  });
</script>

<div class="mapwrap" class:fill bind:this={wrap}>
  <div class="map" bind:this={host} aria-label="Hartă locații"></div>
  {#if fallback}
    <p class="fallback-note" role="status">Dalele OpenStreetMap nu se încarcă — se afișează contururi locale. Se reîncearcă automat.</p>
  {/if}
</div>

<style>
  .mapwrap {
    position: relative;
    height: 420px;
  }
  .mapwrap.fill {
    height: 100%;
  }
.mapwrap:fullscreen {
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
  :global(.nw-marker) {
    position: relative;
  }
  :global(.nw-label) {
    position: absolute;
    visibility: hidden;
    white-space: nowrap;
    pointer-events: none;
    font-size: 12px;
    font-weight: 700;
    line-height: 1.3;
    color: var(--fg);
    text-shadow: -1px -1px 0 var(--surface), 1px -1px 0 var(--surface), -1px 1px 0 var(--surface), 1px 1px 0 var(--surface), 0 0 3px var(--surface);
  }
  :global(.nw-label[class*='pos-']) {
    visibility: visible;
  }
  :global(.nw-label.pos-right) {
    left: 30px;
    top: 50%;
    transform: translateY(-50%);
  }
  :global(.nw-label.pos-left) {
    right: 30px;
    top: 50%;
    transform: translateY(-50%);
  }
  :global(.nw-label.pos-top) {
    bottom: 30px;
    left: 50%;
    transform: translateX(-50%);
  }
  :global(.nw-label.pos-bottom) {
    top: 30px;
    left: 50%;
    transform: translateX(-50%);
  }
  :global(.nw-tools a) {
    font-weight: 700;
    cursor: pointer;
  }
  :global(.nw-tools a:not(.leaflet-disabled)) {
    background: var(--surface);
    color: var(--fg);
  }
  :global(.nw-tools a:not(.leaflet-disabled):hover) {
    background: var(--surface-2);
  }
  :global(.leaflet-tooltip.nw-hint) {
    background: var(--surface);
    color: var(--fg);
    border-color: var(--border);
    font-weight: 600;
  }
  :global(.leaflet-tooltip-top.nw-hint::before) {
    border-top-color: var(--border);
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
  :global(.leaflet-control-attribution) {
    background: var(--surface) !important;
    color: var(--fg);
  }
</style>
