<script lang="ts" module>
  // survives the remount after a new location is created
  let flash = '';
</script>

<script lang="ts">
  import { onMount, tick } from 'svelte';
  import CoordPicker from '../../components/CoordPicker.svelte';
  import DeviceRow from '../../components/DeviceRow.svelte';
  import { ApiError, get, send } from '../../lib/api';
  import { confirmDialog } from '../../lib/confirm.svelte';
  import { emptyDevice, emptyLocation, fieldPath, isDirty, moveDevice, parseCoord, toInput, toPayload } from '../../lib/editor';
  import { beforeLeave, navigate, router } from '../../lib/router.svelte';
  import type { AdminLocation, DeviceInput, LocationInput, Settings } from '../../lib/types';

  const idParam = router.route.params.id;
  const isNew = idParam === 'nou';
  const id = isNew ? null : Number(idParam);

  type Row = DeviceInput & { _key: number };
  let nextKey = 0;
  const keyed = (ds: DeviceInput[]): Row[] => ds.map((d) => ({ ...d, _key: nextKey++ }));

  let form = $state<LocationInput>(emptyLocation());
  let rows = $state<Row[]>(keyed(emptyLocation().devices));
  let baseline = $state<LocationInput>(emptyLocation());
  let latText = $state('');
  let lonText = $state('');
  let defaults = $state<Settings | null>(null);
  let kinds = $state<string[]>([]);
  let loaded = $state(false);
  let notFound = $state(false);
  let busy = $state(false);
  let saved = $state(flash);
  flash = '';
  let error = $state('');
  let fieldErrors = $state<Record<string, string>>({});
  let rowErrors = $state<Record<number, Record<string, string>>>({});
  let dragFrom: number | null = null;

  function current(): LocationInput {
    return { ...form, devices: rows.map(({ _key, ...d }) => d) };
  }

  const dirty = $derived(isDirty({ ...form, devices: rows.map(({ _key, ...d }) => d) }, baseline));

  function setCoordsText() {
    latText = form.lat == null ? '' : String(form.lat).replace('.', ',');
    lonText = form.lon == null ? '' : String(form.lon).replace('.', ',');
  }

  function adopt(l: LocationInput) {
    form = { ...l, devices: [] };
    rows = keyed(l.devices);
    baseline = structuredClone(l);
    setCoordsText();
  }

  async function load() {
    try {
      const [list, s] = await Promise.all([get<{ locations: AdminLocation[] }>('/api/admin/locations'), get<Settings>('/api/admin/settings')]);
      defaults = s;
      kinds = [...new Set(list.locations.flatMap((l) => l.devices.map((d) => d.kind)).filter(Boolean))].sort((a, b) => a.localeCompare(b, 'ro'));
      if (id != null) {
        const l = list.locations.find((x) => x.id === id);
        if (!l) notFound = true;
        else adopt(toInput(l));
      }
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  function coordsFromText(): boolean {
    const a = latText.trim() === '' ? null : parseCoord(latText);
    const b = lonText.trim() === '' ? null : parseCoord(lonText);
    if (latText.trim() !== '' && a === null) {
      fieldErrors = { lat: 'Latitudine invalidă (de exemplu 46,99).' };
      return false;
    }
    if (lonText.trim() !== '' && b === null) {
      fieldErrors = { lon: 'Longitudine invalidă (de exemplu 28,85).' };
      return false;
    }
    form.lat = a;
    form.lon = b;
    return true;
  }

  async function focusFirstInvalid() {
    await tick();
    (document.querySelector('.editor [aria-invalid="true"]') as HTMLElement | null)?.focus();
  }

  async function save(e: Event) {
    e.preventDefault();
    saved = error = '';
    fieldErrors = {};
    rowErrors = {};
    if (!coordsFromText()) return focusFirstInvalid();
    busy = true;
    try {
      const body = toPayload(current());
      const r = isNew ? await send<{ id: number }>('POST', '/api/admin/locations', body) : await send<{ id: number }>('PUT', `/api/admin/locations/${id}`, body);
      baseline = $state.snapshot(current()) as LocationInput;
      if (isNew) {
        flash = 'Locația a fost salvată.';
        await navigate(`/administrare/locatie/${r.id}`, { replace: true, force: true });
        return;
      }
      await load();
      saved = 'Locația a fost salvată.';
    } catch (err) {
      if (err instanceof ApiError && err.field) {
        const p = fieldPath(err.field);
        if (p.device !== undefined) rowErrors = { [p.device]: { [p.name]: err.message } };
        else if (['name', 'address', 'notes', 'lat', 'lon'].includes(p.name)) fieldErrors = { [p.name]: err.message };
        else error = err.message;
      } else error = (err as Error).message;
      focusFirstInvalid();
    } finally {
      busy = false;
    }
  }

  async function cancel() {
    if (dirty && !(await confirmDialog('Aveți modificări nesalvate. Le abandonați?', 'Abandonează', 'Rămân', true))) return;
    baseline = $state.snapshot(current()) as LocationInput;
    navigate('/administrare', { force: true });
  }

  async function archive() {
    const ok = await confirmDialog(
      `Arhivați locația „${form.name}”? Locația și istoricul ei rămân în statistici, dar monitorizarea se oprește.`,
      'Arhivează',
      'Renunță',
      true,
    );
    if (!ok) return;
    try {
      await send('DELETE', `/api/admin/locations/${id}`);
      baseline = $state.snapshot(current()) as LocationInput;
      navigate('/administrare', { force: true });
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function move(from: number, to: number) {
    rows = moveDevice(rows, from, to);
    rowErrors = {};
  }

  function remove(i: number) {
    rows = rows.filter((_, k) => k !== i);
    rowErrors = {};
  }

  function pick(a: number, b: number) {
    form.lat = a;
    form.lon = b;
    setCoordsText();
  }

  function clearCoords() {
    form.lat = null;
    form.lon = null;
    setCoordsText();
  }

  onMount(() => {
    load();
    const off = beforeLeave(async () => !dirty || (await confirmDialog('Aveți modificări nesalvate. Le abandonați?', 'Abandonează', 'Rămân', true)));
    const onUnload = (e: BeforeUnloadEvent) => {
      if (dirty) e.preventDefault();
    };
    window.addEventListener('beforeunload', onUnload);
    return () => {
      off();
      window.removeEventListener('beforeunload', onUnload);
    };
  });
</script>

<p class="crumbs"><a href="/administrare">← Locații</a></p>

{#if notFound}
  <p class="notice">Locația nu există sau a fost arhivată.</p>
{:else if !loaded}
  <p class="muted">Se încarcă…</p>
{:else}
  <form class="editor" onsubmit={save} novalidate>
    <h1>{isNew ? 'Locație nouă' : baseline.name || 'Locație'}</h1>
    {#if error}<p class="notice danger" role="alert">{error}</p>{/if}
    {#if saved}<p class="notice" role="status">{saved}</p>{/if}

    <fieldset class="card general">
      <legend>Date generale</legend>
      <div class="field">
        <label for="loc-name">Denumire</label>
        <input id="loc-name" bind:value={form.name} required maxlength="120" aria-invalid={!!fieldErrors.name} />
        {#if fieldErrors.name}<p class="error-text">{fieldErrors.name}</p>{/if}
      </div>
      <div class="field">
        <label for="loc-addr">Adresă</label>
        <input id="loc-addr" bind:value={form.address} maxlength="500" aria-invalid={!!fieldErrors.address} />
        {#if fieldErrors.address}<p class="error-text">{fieldErrors.address}</p>{/if}
      </div>
      <div class="field">
        <label for="loc-notes">Observații</label>
        <textarea id="loc-notes" rows="2" bind:value={form.notes} maxlength="4000" aria-invalid={!!fieldErrors.notes}></textarea>
        {#if fieldErrors.notes}<p class="error-text">{fieldErrors.notes}</p>{/if}
      </div>
      <div class="coords">
        <div class="field">
          <label for="loc-lat">Latitudine</label>
          <input id="loc-lat" inputmode="decimal" bind:value={latText} onchange={coordsFromText} aria-invalid={!!fieldErrors.lat} />
          {#if fieldErrors.lat}<p class="error-text">{fieldErrors.lat}</p>{/if}
        </div>
        <div class="field">
          <label for="loc-lon">Longitudine</label>
          <input id="loc-lon" inputmode="decimal" bind:value={lonText} onchange={coordsFromText} aria-invalid={!!fieldErrors.lon} />
          {#if fieldErrors.lon}<p class="error-text">{fieldErrors.lon}</p>{/if}
        </div>
        {#if form.lat != null}
          <button type="button" onclick={clearCoords}>Șterge coordonatele</button>
        {/if}
      </div>
      <CoordPicker lat={form.lat} lon={form.lon} onPick={pick} />
    </fieldset>

    <fieldset class="devices">
      <legend>Echipamente</legend>
      {#each rows as row, i (row._key)}
        <DeviceRow
          bind:device={rows[i]}
          index={i}
          count={rows.length}
          errors={rowErrors[i] ?? {}}
          {defaults}
          {kinds}
          onMove={(to) => move(i, to)}
          onRemove={() => remove(i)}
          onDragStart={() => (dragFrom = i)}
          onDrop={() => dragFrom !== null && move(dragFrom, i)}
        />
      {/each}
      <div>
        <button type="button" onclick={() => (rows = [...rows, ...keyed([emptyDevice(false)])])} disabled={rows.length >= 50}>Adaugă echipament</button>
      </div>
    </fieldset>

    <div class="bar">
      <button type="submit" class="primary" disabled={busy}>{busy ? 'Se salvează…' : 'Salvează'}</button>
      <button type="button" onclick={cancel}>Renunță</button>
      {#if dirty}<span class="muted">Modificări nesalvate</span>{/if}
      <span class="spacer"></span>
      {#if !isNew}<button type="button" class="danger" onclick={archive}>Arhivează</button>{/if}
    </div>
  </form>
{/if}

<style>
  .crumbs {
    margin: 0 0 4px;
  }
  .editor h1 {
    margin-top: 0;
    overflow-wrap: anywhere;
  }
  fieldset {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 14px;
    margin: 0 0 14px;
  }
  legend {
    font-weight: 700;
    padding: 0 6px;
  }
  .general input,
  .general textarea {
    width: 100%;
  }
  .coords {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    flex-wrap: wrap;
  }
  .coords input {
    width: 180px;
  }
  .devices {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .bar {
    display: flex;
    gap: 8px;
    align-items: center;
    position: sticky;
    bottom: 0;
    background: var(--bg);
    padding: 10px 0;
    border-top: 1px solid var(--border);
    z-index: 600;
  }
  .spacer {
    flex: 1;
  }
  .danger {
    border-color: var(--c-down);
    color: var(--c-down);
  }
</style>
