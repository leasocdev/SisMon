<script lang="ts">
  import { PRESET_LABEL, PRESETS, type PresetKey } from '../lib/periods';
  import { utcToZonedParts, zonedToUtc } from '../lib/time';

  let {
    preset,
    from,
    to,
    onChange,
  }: {
    preset: PresetKey | 'custom';
    from: number;
    to: number;
    onChange: (p: { preset: PresetKey } | { preset: 'custom'; from: number; to: number }) => void;
  } = $props();

  let open = $state(false);
  let fd = $state('');
  let ft = $state('');
  let td = $state('');
  let tt = $state('');
  let error = $state('');

  function openCustom() {
    const a = utcToZonedParts(from);
    const b = utcToZonedParts(to);
    fd = a.date;
    ft = a.time;
    td = b.date;
    tt = b.time;
    error = '';
    open = true;
  }

  function apply(e: Event) {
    e.preventDefault();
    const a = zonedToUtc(fd, ft || '00:00');
    const b = zonedToUtc(td, tt || '00:00');
    if (Number.isNaN(a) || Number.isNaN(b)) {
      error = 'Completați data și ora pentru început și sfârșit.';
      return;
    }
    if (b <= a) {
      error = 'Sfârșitul trebuie să fie după început.';
      return;
    }
    if (b - a > 400 * 86_400_000) {
      error = 'Perioada poate avea cel mult 400 de zile.';
      return;
    }
    error = '';
    open = false;
    onChange({ preset: 'custom', from: a, to: b });
  }
</script>

<div class="picker">
  <div class="presets" role="group" aria-label="Perioadă">
    {#each PRESETS as p (p)}
      <button type="button" aria-pressed={preset === p} class:on={preset === p} onclick={() => onChange({ preset: p })}>{PRESET_LABEL[p]}</button>
    {/each}
    <button type="button" aria-pressed={preset === 'custom'} aria-expanded={open} class:on={preset === 'custom'} onclick={openCustom}>Personalizată</button>
  </div>
  {#if open}
    <form class="custom" onsubmit={apply}>
      <label>Început — data <input type="date" bind:value={fd} aria-label="Început — data" /></label>
      <label>Început — ora <input type="time" bind:value={ft} aria-label="Început — ora" /></label>
      <label>Sfârșit — data <input type="date" bind:value={td} aria-label="Sfârșit — data" /></label>
      <label>Sfârșit — ora <input type="time" bind:value={tt} aria-label="Sfârșit — ora" /></label>
      <button type="submit" class="primary">Aplică</button>
      <button type="button" onclick={() => (open = false)}>Renunță</button>
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    </form>
  {/if}
</div>

<style>
  .presets {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .on {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    font-weight: 600;
  }
  .custom {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 12px;
    align-items: flex-end;
    margin-top: 8px;
    padding: 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  .custom label {
    display: flex;
    flex-direction: column;
    font-size: 0.88em;
    gap: 2px;
  }
  .error-text {
    flex-basis: 100%;
  }
</style>
