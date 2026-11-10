<script lang="ts">
  import { onMount } from 'svelte';
  import { live } from '../lib/live.svelte';
  import { stateText } from '../lib/status';

  let messages = $state<string[]>([]);

  onMount(() =>
    live.onChange((prev, next) => {
      const msg = next ? `${next.name}: ${stateText(next.state)}` : prev ? `${prev.name}: eliminată din monitorizare` : '';
      if (msg) messages = [...messages, msg].slice(-3);
    }),
  );
</script>

<div class="sr-only" aria-live="polite" aria-atomic="false">
  {#each messages as m, i (i + m)}
    <p>{m}</p>
  {/each}
</div>
