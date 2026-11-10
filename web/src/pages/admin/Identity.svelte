<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, get, send, upload } from '../../lib/api';
  import { confirmDialog } from '../../lib/confirm.svelte';
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

  let logoFile = $state<File | null>(null);
  let logoError = $state('');

  function adopt(r: Identity) {
    id = r;
    live.identity = r;
  }

  async function uploadLogo() {
    if (!logoFile) return;
    logoError = '';
    try {
      adopt(await upload<Identity>('/api/admin/identity/logo', logoFile));
    } catch (e) {
      logoError = (e as Error).message;
    }
  }

  async function removeLogo() {
    if (!(await confirmDialog('Eliminați logo-ul?', 'Elimină', 'Renunță', true))) return;
    try {
      adopt(await send<Identity>('DELETE', '/api/admin/identity/logo'));
    } catch (e) {
      logoError = (e as Error).message;
    }
  }

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

{#if id}
  <section class="card ident logo-box">
    <h2>Logo</h2>
    {#if id.logo_version > 0}
      <img class="preview" src="/branding/logo?v={id.logo_version}" alt="Logo-ul curent" />
    {:else}
      <p class="muted">Nu este încărcat niciun logo.</p>
    {/if}
    <p class="muted">PNG, cel mult 1024 × 1024 pixeli și 500 KB. Apare în antet, pe ecranul de dispecerat, în Raportul 24H și în exportul PDF.</p>
    <div class="row">
      <label class="file">Fișier PNG <input type="file" accept="image/png" onchange={(e) => (logoFile = e.currentTarget.files?.[0] ?? null)} /></label>
      <button type="button" onclick={uploadLogo} disabled={!logoFile}>Încarcă logo-ul</button>
      {#if id.logo_version > 0}<button type="button" class="danger" onclick={removeLogo}>Elimină logo-ul</button>{/if}
    </div>
    {#if logoError}<p class="error-text" role="alert">{logoError}</p>{/if}
  </section>
{/if}

<style>
  .ident {
    padding: 14px 16px;
    max-width: 640px;
  }
  .ident input {
    width: 100%;
  }
  .logo-box {
    margin-top: 14px;
  }
  .logo-box h2 {
    font-size: 1.05em;
    margin-top: 0;
  }
  .preview {
    max-height: 80px;
    max-width: 320px;
    background: var(--surface-2);
    padding: 6px;
    border-radius: var(--radius);
  }
  .row {
    display: flex;
    gap: 10px;
    align-items: flex-end;
    flex-wrap: wrap;
  }
  .file {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-weight: 600;
  }
  .danger {
    border-color: var(--c-down);
    color: var(--c-down);
  }
  .ok {
    margin-left: 10px;
    color: var(--c-up);
    font-weight: 600;
  }
</style>
