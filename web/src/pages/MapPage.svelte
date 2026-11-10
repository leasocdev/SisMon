<script lang="ts">
  import MapView from '../components/MapView.svelte';
</script>

<div class="mappage">
  <h1 class="sr-only">Hartă</h1>
  <MapView />
</div>

<style>
  .mappage {
    height: calc(100vh - 64px);
    min-height: 360px;
    display: flex;
    flex-direction: column;
    padding: 8px;
  }
</style>
