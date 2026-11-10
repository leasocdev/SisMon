<script lang="ts">
  import { onMount } from 'svelte';
  import type * as Leaflet from 'leaflet';
  import { loadLeaflet } from '../lib/leaflet';

  let { lat, lon, onPick }: { lat: number | null; lon: number | null; onPick: (lat: number, lon: number) => void } = $props();

  let host: HTMLDivElement;
  let L: typeof Leaflet | null = null;
  let map: Leaflet.Map | null = null;
  let marker: Leaflet.Marker | null = null;

  const round = (v: number) => Math.round(v * 1e6) / 1e6;

  function pinIcon(lib: typeof Leaflet) {
    const s = document.createElement('span');
    s.className = 'coord-pin';
    s.textContent = '●';
    return lib.divIcon({ html: s, className: 'nw-icon', iconSize: [22, 22], iconAnchor: [11, 11] });
  }

  function place(a: number, b: number) {
    if (!L || !map) return;
    if (!marker) {
      marker = L.marker([a, b], { draggable: true, icon: pinIcon(L), keyboard: false }).addTo(map);
      marker.on('dragend', () => {
        const p = marker!.getLatLng();
        onPick(round(p.lat), round(p.lng));
      });
    } else marker.setLatLng([a, b]);
  }

  onMount(() => {
    let alive = true;
    loadLeaflet().then((lib) => {
      if (!alive) return;
      L = lib;
      const has = lat != null && lon != null;
      map = lib.map(host, { minZoom: 2, maxZoom: 18 }).setView(has ? [lat!, lon!] : [47.0, 28.6], has ? 13 : 7);
      map.attributionControl.setPrefix('<a href="https://leafletjs.com">Leaflet</a>');
      lib.tileLayer('/tiles/{z}/{x}/{y}.png', { minZoom: 2, maxZoom: 18, attribution: '© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>' }).addTo(map);
      map.on('click', (e: Leaflet.LeafletMouseEvent) => {
        const a = round(e.latlng.lat);
        const b = round(e.latlng.lng);
        place(a, b);
        onPick(a, b);
      });
      if (has) place(lat!, lon!);
    });
    return () => {
      alive = false;
      map?.stop();
      map?.off();
      map?.remove();
    };
  });

  $effect(() => {
    if (lat != null && lon != null) place(lat, lon);
    else if (marker) {
      marker.remove();
      marker = null;
    }
  });
</script>

<div class="coord-map">
  <div class="m" bind:this={host} aria-label="Hartă pentru alegerea coordonatelor: faceți click pe poziția locației"></div>
  <p class="muted hint">Faceți click pe hartă pentru a seta coordonatele; marcajul se poate trage.</p>
</div>

<style>
  .m {
    height: 260px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .hint {
    font-size: 0.85em;
    margin: 4px 0 0;
  }
  :global(.coord-pin) {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    color: var(--c-down);
    font-size: 22px;
    line-height: 1;
    cursor: grab;
  }
</style>
