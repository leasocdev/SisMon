<script lang="ts">
  import { onMount } from 'svelte';
  import MapView from '../components/MapView.svelte';

  let box: HTMLDivElement;
  let top = $state(56);

  // The map covers the whole browser window below the header (and demo banner).
  onMount(() => {
    const measure = () => (top = Math.max(0, Math.round(box.parentElement!.getBoundingClientRect().top + window.scrollY)));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(document.body);
    window.addEventListener('resize', measure);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', measure);
    };
  });
</script>

<div class="mappage" bind:this={box} style:top="{top}px">
  <h1 class="sr-only">Hartă</h1>
  <MapView />
</div>

<style>
  .mappage {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
  }
</style>
