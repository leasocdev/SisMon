<script lang="ts">
  import { onMount } from 'svelte';
  import { live } from '../lib/live.svelte';
  import { STATE_PLURAL, STATE_SYMBOL, STATES, totals, type SortMode } from '../lib/status';
  import type { State } from '../lib/types';

  let {
    filter = $bindable(),
    query = $bindable(),
    sort = $bindable(),
  }: { filter: Set<State>; query: string; sort: SortMode } = $props();

  // „Funcționale" primul în bara de filtre.
  const ORDER: State[] = ['up', ...STATES.filter((s) => s !== 'up')];
  const t = $derived(totals(live.locations));
  const warning = $derived((live.tick, live.warning()));
  let search: HTMLInputElement;

  function toggle(s: State) {
    const next = new Set(filter);
    if (next.has(s)) next.delete(s);
    else next.add(s);
    filter = next;
  }

  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement;
      if (e.key === '/' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)) {
        e.preventDefault();
        search.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  });
</script>

<div class="topbar">
  <div class="totals" role="group" aria-label="Filtru după stare">
    {#each ORDER as s (s)}
      <button type="button" class="tot" class:on={filter.has(s)} aria-pressed={filter.has(s)} onclick={() => toggle(s)}>
        <span class="sym f-{s}" aria-hidden="true">{STATE_SYMBOL[s]}</span>
        <span class="n">{t[s]}</span>
        <span class="lbl">{STATE_PLURAL[s]}</span>
      </button>
    {/each}
  </div>
  <label class="search">
    <span class="sr-only">Căutare (nume, echipament, IP)</span>
    <input bind:this={search} type="search" placeholder="Căutare (nume, IP) — tasta /" bind:value={query} aria-label="Căutare (nume, echipament, IP)" />
  </label>
  <label class="sort">
    Ordine
    <select bind:value={sort}>
      <option value="severity">după gravitate</option>
      <option value="alpha">alfabetic</option>
      <option value="manual">manuală</option>
    </select>
  </label>
</div>
{#if warning}
  <div class="notice warn" role="alert">⚠ {warning}</div>
{/if}
{#if live.loadError}
  <div class="notice danger" role="alert">{live.loadError}</div>
{/if}

<style>
  .topbar {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
    margin-bottom: 18px;
    padding: 10px 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-bottom: 3px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 2px 6px rgb(0 0 0 / 0.12);
  }
  .totals {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .tot {
    display: inline-flex;
    gap: 6px;
    align-items: center;
  }
  .tot.on {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    font-weight: 600;
  }
  .sym {
    display: inline-flex;
    width: 1.4em;
    height: 1.4em;
    border-radius: 50%;
    align-items: center;
    justify-content: center;
    font-size: 0.85em;
    font-weight: 700;
  }
  .n {
    font-weight: 700;
  }
  .search input {
    width: 260px;
    max-width: 100%;
  }
  .sort {
    margin-left: auto;
    display: inline-flex;
    gap: 6px;
    align-items: center;
    font-size: 0.92em;
  }
</style>
