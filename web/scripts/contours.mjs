// Builds src/assets/contours.json: Natural Earth 1:50m country outlines
// (public domain) for Moldova and its neighbours, used as the offline map
// fallback. Run once: node scripts/contours.mjs
import { writeFileSync } from 'node:fs';

const SRC = 'https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_50m_admin_0_countries.geojson';
const BOX = { minLon: 20, maxLon: 35, minLat: 43, maxLat: 50 };

const round = (c) => (typeof c[0] === 'number' ? [Math.round(c[0] * 1000) / 1000, Math.round(c[1] * 1000) / 1000] : c.map(round));

function touches(coords) {
  if (typeof coords[0] === 'number') {
    const [lon, lat] = coords;
    return lon >= BOX.minLon && lon <= BOX.maxLon && lat >= BOX.minLat && lat <= BOX.maxLat;
  }
  return coords.some(touches);
}

const res = await fetch(SRC);
if (!res.ok) throw new Error('download: ' + res.status);
const src = await res.json();
const features = src.features
  .filter((f) => touches(f.geometry.coordinates))
  .map((f) => ({
    type: 'Feature',
    properties: { name: f.properties.NAME_RO || f.properties.NAME },
    geometry: { type: f.geometry.type, coordinates: round(f.geometry.coordinates) },
  }));
const out = JSON.stringify({ type: 'FeatureCollection', features });
writeFileSync(new URL('../src/assets/contours.json', import.meta.url), out);
console.log(features.map((f) => f.properties.name).join(', '), '—', (out.length / 1024).toFixed(0), 'KB');
