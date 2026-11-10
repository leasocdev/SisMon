<script lang="ts">
  import { live } from '../lib/live.svelte';

  let { large = false }: { large?: boolean } = $props();

  const status = $derived(live.stale ? 'stale' : live.connected ? 'ok' : 'retry');
  const text = $derived(status === 'stale' ? 'Date neactualizate' : status === 'ok' ? 'Conectat' : 'Reconectare…');
</script>

<span class="badge {status}" class:large role="status" aria-live="polite">
  <span class="dot" aria-hidden="true"></span>{text}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    border-radius: 999px;
    border: 1px solid var(--border);
    font-size: 0.88em;
    white-space: nowrap;
  }
  .large {
    font-size: 1.1em;
    padding: 5px 14px;
  }
  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--c-unknown);
  }
  .ok .dot {
    background: var(--c-up);
  }
  .retry .dot {
    background: var(--c-degraded);
  }
  .stale {
    border-color: var(--c-unknown);
    font-weight: 600;
  }
</style>
