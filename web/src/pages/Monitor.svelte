<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import LocationCard from '../components/LocationCard.svelte';
  import TopBar from '../components/TopBar.svelte';
  import { live } from '../lib/live.svelte';
  import { filterLocations, sortLocations, type SortMode } from '../lib/status';
  import { timelines } from '../lib/timelines.svelte';
  import type { State } from '../lib/types';

  let { panel }: { panel?: Snippet } = $props();

  function storedSort(): SortMode {
    try {
      const v = localStorage.getItem('nw-sort');
      if (v === 'alpha' || v === 'manual' || v === 'severity') return v;
    } catch {
      /* ignore */
    }
    return 'severity';
  }

  let filter = $state<Set<State>>(new Set());
  let query = $state('');
  let sort = $state<SortMode>(storedSort());

  $effect(() => {
    try {
      localStorage.setItem('nw-sort', sort);
    } catch {
      /* ignore */
    }
  });

  const shown = $derived(sortLocations(filterLocations(live.locations, filter, query), sort));

  onMount(() => timelines.acquire());
</script>

<div class="page monitor">
  <h1 class="sr-only">Monitorizare</h1>
  <TopBar bind:filter bind:query bind:sort />
  {#if live.loaded && live.locations.length === 0}
    <p class="notice">Nu există locații configurate. <a href="/administrare">Adăugați locații din Administrare.</a></p>
  {:else if live.loaded && shown.length === 0}
    <p class="notice">Nicio locație nu corespunde filtrului.</p>
  {/if}
  <div class="grid">
    {#each shown as loc (loc.id)}
      <LocationCard {loc} />
    {/each}
  </div>
</div>
{@render panel?.()}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(max(300px, calc((100% - 3 * var(--gap)) / 4)), 1fr));
    gap: var(--gap);
  }
</style>
