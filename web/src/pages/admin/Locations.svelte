<script lang="ts">
  import { onMount } from 'svelte';
  import StatusBadge from '../../components/StatusBadge.svelte';
  import { get, send } from '../../lib/api';
  import { confirmDialog } from '../../lib/confirm.svelte';
  import { moveDevice } from '../../lib/editor';
  import { live } from '../../lib/live.svelte';
  import { fold } from '../../lib/status';
  import { fmtDateTime } from '../../lib/time';
  import type { AdminLocation, ArchivedLocation } from '../../lib/types';

  let locations = $state<AdminLocation[]>([]);
  let query = $state('');
  let error = $state('');
  let saved = $state('');
  let loaded = $state(false);
  let archived = $state<ArchivedLocation[]>([]);
  let dragFrom: number | null = null;

  const filtering = $derived(query.trim() !== '');
  const shown = $derived(
    locations.filter((l) => {
      const q = fold(query.trim());
      if (!q) return true;
      return fold(l.name).includes(q) || l.devices.some((d) => fold(d.name).includes(q) || d.host.toLowerCase().includes(q));
    }),
  );

  async function loadArchived() {
    try {
      archived = (await get<{ locations: ArchivedLocation[] }>('/api/admin/locations/archived')).locations;
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function purge(l: ArchivedLocation) {
    const ok = await confirmDialog(
      `Ștergeți definitiv locația arhivată „${l.name}”? Se șterg și echipamentele ei, împreună cu întregul istoric (măsurători, incidente, statistici). Operația este ireversibilă.`,
      'Șterge definitiv',
      'Renunță',
      true,
      l.name,
    );
    if (!ok) return;
    try {
      await send('DELETE', `/api/admin/locations/${l.id}/permanent`);
      saved = `Locația „${l.name}” a fost ștearsă definitiv.`;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
    await loadArchived();
  }

  async function load() {
    try {
      locations = (await get<{ locations: AdminLocation[] }>('/api/admin/locations')).locations;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  let busy = $state(false);
  async function move(from: number, to: number) {
    if (filtering || busy) return; // reorder only the full list, one request at a time
    const next = moveDevice(locations, from, to);
    if (next === locations) return;
    locations = next;
    busy = true;
    try {
      await send('PUT', '/api/admin/locations/order', { ids: next.map((l) => l.id) });
      saved = 'Ordinea a fost salvată.';
      error = '';
    } catch (e) {
      error = (e as Error).message;
      await load(); // the server's order is the truth
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    load();
    loadArchived();
  });
</script>

<div class="head">
  <h1>Locații</h1>
  <a class="btn primary" href="/administrare/locatie/nou">Adaugă locație</a>
</div>
<label class="search">
  <span class="sr-only">Căutare locații</span>
  <input type="search" bind:value={query} placeholder="Căutare după nume sau IP" aria-label="Căutare locații" />
</label>
{#if error}<p class="notice danger" role="alert">{error}</p>{/if}
{#if saved}<p class="muted" role="status">{saved}</p>{/if}
{#if filtering}<p class="muted">Reordonarea este disponibilă doar fără filtru de căutare.</p>{/if}

{#if loaded && locations.length === 0}
  <p class="notice">Nu există locații. Adăugați prima locație.</p>
{/if}
<ul class="loc-list">
  {#each shown as l, i (l.id)}
    {@const lv = live.location(l.id)}
    <li class="card" ondragover={(e) => e.preventDefault()} ondrop={(e) => (e.preventDefault(), dragFrom !== null && move(dragFrom, i))}>
      <span class="handle" draggable={!filtering} ondragstart={() => (dragFrom = i)} aria-hidden="true" title="Trageți pentru a reordona">⠿</span>
      <a href="/administrare/locatie/{l.id}">{l.name}</a>
      <span class="muted">{l.devices.length} {l.devices.length === 1 ? 'echipament' : 'echipamente'}</span>
      {#if l.devices.some((d) => d.disabled)}
        {@const off = l.devices.filter((d) => d.disabled).length}
        <span class="off-tag" title="Echipamente care nu se monitorizează temporar">{off} {off === 1 ? 'dezactivat' : 'dezactivate'}</span>
      {/if}
      {#if lv}<StatusBadge state={lv.state} size="small" />{/if}
      <span class="spacer"></span>
      <button type="button" onclick={() => move(i, i - 1)} disabled={i === 0 || filtering || busy} aria-label="Mută în sus">↑</button>
      <button type="button" onclick={() => move(i, i + 1)} disabled={i === shown.length - 1 || filtering || busy} aria-label="Mută în jos">↓</button>
    </li>
  {/each}
</ul>

{#if archived.length > 0}
  <details class="archived">
    <summary>Locații arhivate ({archived.length})</summary>
    <p class="muted">Nu se monitorizează. Istoricul lor apare în Statistici doar cu opțiunea „Include arhivate”.</p>
    <ul class="arch-list">
      {#each archived as l (l.id)}
        <li class="card">
          <span class="name">{l.name}</span>
          <span class="muted">{l.devices} {l.devices === 1 ? 'echipament' : 'echipamente'}</span>
          <span class="muted">arhivată la {fmtDateTime(l.archived_at)}</span>
          <span class="spacer"></span>
          <button type="button" class="danger" onclick={() => purge(l)} aria-label="Șterge definitiv {l.name}">Șterge definitiv</button>
        </li>
      {/each}
    </ul>
  </details>
{/if}

<style>
  .archived {
    margin-top: 24px;
  }
  .archived summary {
    cursor: pointer;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .archived .name {
    font-weight: 600;
  }
  .danger {
    border-color: var(--c-down);
    color: var(--c-down);
  }
  .off-tag {
    border: 1px dashed var(--muted);
    border-radius: 3px;
    padding: 0 6px;
    font-size: 0.85em;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }
  .btn {
    display: inline-block;
    padding: 6px 14px;
    border-radius: var(--radius);
    text-decoration: none;
  }
  .btn.primary {
    background: var(--focus);
    color: var(--surface);
  }
  .search input {
    width: 320px;
    max-width: 100%;
    margin-bottom: 10px;
  }
  .loc-list,
  .arch-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .loc-list li,
  .arch-list li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 10px;
  }
  .loc-list a {
    font-weight: 600;
  }
  .handle {
    cursor: grab;
    color: var(--muted);
    user-select: none;
  }
  .spacer {
    flex: 1;
  }
</style>
