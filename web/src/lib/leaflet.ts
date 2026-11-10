// Loads Leaflet and the marker-cluster plugin once, on demand.
import type * as Leaflet from 'leaflet';

let loading: Promise<typeof Leaflet> | null = null;

export function loadLeaflet(): Promise<typeof Leaflet> {
  loading ??= (async () => {
    const mod = await import('leaflet');
    const L = (mod.default ?? mod) as typeof Leaflet;
    // the cluster plugin extends the global L
    (globalThis as unknown as { L: typeof Leaflet }).L = L;
    await Promise.all([
      import('leaflet/dist/leaflet.css'),
      import('leaflet.markercluster/dist/MarkerCluster.css'),
      import('leaflet.markercluster'),
    ]);
    return L;
  })();
  loading.catch(() => (loading = null)); // allow a retry after a failed download
  return loading;
}
