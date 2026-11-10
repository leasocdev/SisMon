<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, get, send } from '../../lib/api';
  import { live } from '../../lib/live.svelte';
  import type { Identity } from '../../lib/types';

  let id = $state<Identity | null>(null);
  let errors = $state<Record<string, string>>({});
  let error = $state('');
  let saved = $state('');

  onMount(async () => {
    try {
      id = await get<Identity>('/api/admin/identity');
    } catch (e) {
      error = (e as Error).message;
    }
  });

  async function save(e: Event) {
    e.preventDefault();
    if (!id) return;
    errors = {};
    error = saved = '';
    try {
      const r = await send<Identity>('PUT', '/api/admin/identity', { name: id.name, subtitle: id.subtitle, report_title: id.report_title });
      id = r;
      live.identity = r;
      saved = 'Identitatea a fost salvată.';
    } catch (err) {
      if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
      else error = (err as Error).message;
    }
  }
</script>

<h1>Identitate</h1>
{#if error}<p class="notice danger" role="alert">{error}</p>{/if}
{#if id}
  <form class="card ident" onsubmit={save} novalidate>
    <div class="field">
      <label for="id-name">Denumirea aplicației</label>
      <input id="id-name" bind:value={id.name} maxlength="60" aria-invalid={!!errors.name} />
      {#if errors.name}<p class="error-text">{errors.name}</p>{/if}
    </div>
    <div class="field">
      <label for="id-sub">Subtitlu</label>
      <input id="id-sub" bind:value={id.subtitle} maxlength="80" aria-invalid={!!errors.subtitle} />
      {#if errors.subtitle}<p class="error-text">{errors.subtitle}</p>{/if}
    </div>
    <div class="field">
      <label for="id-rep">Titlul raportului</label>
      <input id="id-rep" bind:value={id.report_title} maxlength="120" aria-invalid={!!errors.report_title} />
      {#if errors.report_title}<p class="error-text">{errors.report_title}</p>{/if}
    </div>
    <button type="submit" class="primary">Salvează</button>
    {#if saved}<span class="ok" role="status">{saved}</span>{/if}
  </form>
{/if}

<style>
  .ident {
    padding: 14px 16px;
    max-width: 640px;
  }
  .ident input {
    width: 100%;
  }
  .ok {
    margin-left: 10px;
    color: var(--c-up);
    font-weight: 600;
  }
</style>
