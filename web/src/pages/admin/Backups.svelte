<script lang="ts">
  import { onMount } from 'svelte';
  import { get, send } from '../../lib/api';
  import { backupKind, fmtBytes } from '../../lib/format';
  import { live } from '../../lib/live.svelte';
  import { fmtDateTime } from '../../lib/time';
  import type { BackupInfo } from '../../lib/types';

  let list = $state<BackupInfo[]>([]);
  let error = $state('');
  let busy = $state(false);
  let created = $state('');

  const sorted = $derived([...list].sort((a, b) => b.created - a.created));

  async function load() {
    try {
      list = (await get<{ backups: BackupInfo[] }>('/api/admin/backups')).backups;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function create() {
    busy = true;
    created = '';
    try {
      const b = await send<BackupInfo>('POST', '/api/admin/backups');
      created = `Backup creat: ${b.name}`;
      await load();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  onMount(load);
</script>

<h1>Backup-uri</h1>
<div class="status card">
  <p>
    Ultimul backup automat:
    <strong>{live.health.last_backup ? fmtDateTime(live.health.last_backup) : 'încă niciunul'}</strong>
  </p>
  {#if live.health.backup_error}<p class="notice danger">{live.health.backup_error}</p>{/if}
  <p class="muted">
    Backup-ul automat rulează zilnic (ora din Setări monitorizare); se păstrează 7 zilnice, 4 săptămânale și 12 lunare.
    Restaurarea se face pe server: <code>netwatch restore FIȘIER</code>
  </p>
  <button type="button" class="primary" onclick={create} disabled={busy}>{busy ? 'Se creează…' : 'Creează backup acum'}</button>
  {#if created}<span class="ok" role="status">{created}</span>{/if}
</div>
{#if error}<p class="notice danger" role="alert">{error}</p>{/if}

{#if sorted.length === 0}
  <p class="muted">Nu există backup-uri.</p>
{:else}
  <table class="backups">
    <thead><tr><th scope="col">Fișier</th><th scope="col">Tip</th><th scope="col">Creat</th><th scope="col">Dimensiune</th><th scope="col"><span class="sr-only">Acțiuni</span></th></tr></thead>
    <tbody>
      {#each sorted as b (b.name)}
        <tr>
          <td class="mono">{b.name}</td>
          <td>{backupKind(b.kind)}</td>
          <td>{fmtDateTime(b.created)}</td>
          <td>{fmtBytes(b.size_bytes)}</td>
          <td><a href="/api/admin/backups/{encodeURIComponent(b.name)}" download>Descarcă</a></td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .status {
    padding: 10px 14px;
    margin-bottom: 12px;
  }
  .status p {
    margin: 4px 0 8px;
  }
  .mono {
    font-family: var(--mono);
  }
  .ok {
    margin-left: 10px;
    color: var(--c-up);
    font-weight: 600;
  }
</style>
