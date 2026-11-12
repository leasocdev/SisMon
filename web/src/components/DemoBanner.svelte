<script lang="ts">
  import { live } from '../lib/live.svelte';
</script>

{#if live.demo}
  <div class="demo" role="note">MOD DEMONSTRATIV — date simulate. Parola de administrare: <strong>sismon-demo</strong></div>
{/if}

<style>
  .demo {
    background: var(--c-degraded);
    color: var(--on-degraded);
    text-align: center;
    font-weight: 700;
    padding: 3px 8px;
    letter-spacing: 0.03em;
    position: relative;
    z-index: 1100;
  }
</style>
