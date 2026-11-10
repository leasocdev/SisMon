<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ApiError, get, send } from '../../lib/api';
  import type { Settings } from '../../lib/types';

  let s = $state<Settings | null>(null);
  let errors = $state<Record<string, string>>({});
  let error = $state('');
  let saved = $state('');
  let busy = $state(false);

  const NUMS: { key: keyof Settings; label: string; min: number; max: number; hint?: string }[] = [
    { key: 'interval_s', label: 'Interval între verificări (s)', min: 10, max: 3600 },
    { key: 'packets', label: 'Pachete per verificare', min: 1, max: 10 },
    { key: 'timeout_s', label: 'Timeout per pachet (s)', min: 1, max: 10 },
    { key: 'failures', label: 'Eșecuri pentru confirmarea căderii', min: 1, max: 20 },
    { key: 'recoveries', label: 'Reușite pentru confirmarea revenirii', min: 1, max: 20 },
    { key: 'stale_s', label: 'Expirarea datelor (s)', min: 30, max: 86400, hint: 'Fără verificări mai mult de atât, starea devine „Fără date”.' },
    { key: 'backup_hour', label: 'Ora backup-ului (0–23)', min: 0, max: 23 },
  ];

  onMount(async () => {
    try {
      s = await get<Settings>('/api/admin/settings');
    } catch (e) {
      error = (e as Error).message;
    }
  });

  async function save(e: Event) {
    e.preventDefault();
    if (!s) return;
    busy = true;
    errors = {};
    error = saved = '';
    try {
      s = await send<Settings>('PUT', '/api/admin/settings', $state.snapshot(s));
      saved = 'Setări salvate.';
    } catch (err) {
      if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
      else error = (err as Error).message;
      await tick();
      (document.querySelector('.settings [aria-invalid="true"]') as HTMLElement | null)?.focus();
    } finally {
      busy = false;
    }
  }

  function setNum(k: keyof Settings, v: string) {
    if (!s) return;
    (s as unknown as Record<string, unknown>)[k] = v === '' ? NaN : Number(v);
  }
</script>

<h1>Setări monitorizare</h1>
{#if error}<p class="notice danger" role="alert">{error}</p>{/if}
{#if s}
  <form class="settings card" onsubmit={save} novalidate>
    <div class="grid">
      {#each NUMS as n (n.key)}
        <div class="field">
          <label for="set-{n.key}">{n.label}</label>
          <input id="set-{n.key}" type="number" min={n.min} max={n.max} value={Number.isNaN(s[n.key] as number) ? '' : s[n.key]}
            oninput={(e) => setNum(n.key, e.currentTarget.value)} aria-invalid={!!errors[n.key]} />
          {#if errors[n.key]}<p class="error-text">{errors[n.key]}</p>{:else if n.hint}<p class="muted hint">{n.hint}</p>{/if}
        </div>
      {/each}
      <div class="field">
        <label for="set-inc">Incidente pentru</label>
        <select id="set-inc" bind:value={s.incidents_for} aria-invalid={!!errors.incidents_for}>
          <option value="all">toate echipamentele</option>
          <option value="primary">doar primare</option>
        </select>
        {#if errors.incidents_for}<p class="error-text">{errors.incidents_for}</p>{/if}
      </div>
      <div class="field">
        <label for="set-dir">Al doilea director de backup (opțional)</label>
        <input id="set-dir" bind:value={s.backup_secondary_dir} placeholder="de exemplu \\server\backup sau /mnt/backup" aria-invalid={!!errors.backup_secondary_dir} />
        {#if errors.backup_secondary_dir}<p class="error-text">{errors.backup_secondary_dir}</p>{/if}
      </div>
      <label class="check"><input type="checkbox" bind:checked={s.prefer_ipv4} /> Preferă IPv4 la rezolvarea numelor DNS</label>
    </div>
    <p class="muted">Intervalul trebuie să acopere toate pachetele cu timeout-ul lor; expirarea trebuie să fie cel puțin interval × (prag + 1). Serverul verifică și explică orice nepotrivire.</p>
    <button type="submit" class="primary" disabled={busy}>Salvează</button>
    {#if saved}<span class="ok" role="status">{saved}</span>{/if}
  </form>
{:else if !error}
  <p class="muted">Se încarcă…</p>
{/if}

<style>
  .settings {
    padding: 14px 16px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 4px 18px;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .hint {
    font-size: 0.82em;
    margin: 2px 0 0;
  }
  .ok {
    margin-left: 10px;
    color: var(--c-up);
    font-weight: 600;
  }
</style>
