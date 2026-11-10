<script lang="ts">
  import { live } from '../../lib/live.svelte';
</script>

<h1>Despre</h1>
<div class="card about">
  <p><strong>NetWatch</strong> {live.version} — monitorizarea disponibilității echipamentelor de rețea prin ICMP.</p>
  <p>Toate orele sunt afișate în fusul orar Europe/Chisinau.</p>
  <p>Ghidul de instalare și operare se află în documentația livrată împreună cu aplicația (<code>docs/operare.md</code>).</p>
  <h2>Componente și licențe</h2>
  <ul>
    <li>Svelte — licență MIT</li>
    <li>Leaflet — licență BSD-2-Clause; Leaflet.markercluster — licență MIT</li>
    <li>uPlot — licență MIT</li>
    <li>Date cartografice © OpenStreetMap contributors, licență ODbL</li>
    <li>Contururi de rezervă: Natural Earth — domeniu public</li>
    <li>Go, modernc.org/sqlite, kardianos/service și bibliotecile golang.org/x — licențe BSD/MIT</li>
  </ul>
</div>

<style>
  .about {
    padding: 14px 18px;
    max-width: 760px;
  }
  h2 {
    font-size: 1.05em;
  }
</style>
