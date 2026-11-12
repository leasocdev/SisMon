// Pure helpers for the location editor.

import type { AdminLocation, DeviceInput, LocationInput, Overrides } from './types';

export function emptyDevice(primary = false): DeviceInput {
  return { name: '', host: '', kind: '', is_primary: primary, overrides: {}, disabled: false, disabled_until: null };
}

export function emptyLocation(): LocationInput {
  return { name: '', address: '', notes: '', lat: null, lon: null, devices: [emptyDevice(true)] };
}

export function toInput(l: AdminLocation): LocationInput {
  return {
    name: l.name,
    address: l.address,
    notes: l.notes,
    lat: l.lat,
    lon: l.lon,
    devices: l.devices.map((d) => ({
      id: d.id,
      name: d.name,
      host: d.host,
      kind: d.kind,
      is_primary: d.is_primary,
      overrides: { ...d.overrides },
      disabled: d.disabled,
      disabled_until: d.disabled_until,
    })),
  };
}

function cleanOverrides(o: Overrides): Overrides {
  const out: Overrides = {};
  for (const k of ['interval_s', 'failures', 'recoveries'] as const) {
    const v = o[k];
    if (typeof v === 'number' && Number.isFinite(v)) out[k] = v;
  }
  return out;
}

/** The JSON body for POST/PUT /api/admin/locations. */
export function toPayload(x: LocationInput): LocationInput {
  return {
    name: x.name.trim(),
    address: x.address.trim(),
    notes: x.notes.trim(),
    lat: x.lat,
    lon: x.lon,
    devices: x.devices.map((d) => {
      const o: DeviceInput = {
        name: d.name.trim(),
        host: d.host.trim(),
        kind: d.kind.trim(),
        is_primary: d.is_primary,
        overrides: cleanOverrides(d.overrides),
        disabled: d.disabled,
        disabled_until: d.disabled ? d.disabled_until : null,
      };
      if (d.id !== undefined) o.id = d.id;
      return o;
    }),
  };
}

export function isDirty(a: LocationInput, b: LocationInput): boolean {
  return JSON.stringify(toPayload(a)) !== JSON.stringify(toPayload(b));
}

/** True when every primary device is disabled: the location then has no state. */
export function primariesDisabled(devices: Pick<DeviceInput, 'is_primary' | 'disabled'>[]): boolean {
  const prim = devices.filter((d) => d.is_primary);
  return prim.length > 0 && prim.every((d) => d.disabled);
}

export function moveDevice<T>(list: T[], from: number, to: number): T[] {
  if (from < 0 || from >= list.length || to < 0 || to >= list.length) return list;
  const out = [...list];
  const [x] = out.splice(from, 1);
  out.splice(to, 0, x);
  return out;
}

/** Splits a server error field: 'devices.2.host' → {device: 2, name: 'host'}. */
export function fieldPath(field: string): { device?: number; name: string } {
  const m = /^devices\.(\d+)\.(.+)$/.exec(field);
  if (m) return { device: Number(m[1]), name: m[2] };
  return { name: field };
}

/** Parses a coordinate typed with a dot or a comma; null when empty or invalid. */
export function parseCoord(text: string): number | null {
  const t = text.trim().replace(',', '.');
  if (!t || !/^-?\d+(\.\d+)?$/.test(t)) return null;
  return Number(t);
}
