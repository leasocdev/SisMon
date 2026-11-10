<script lang="ts">
  import { ApiError, upload } from '../../lib/api';
  import type { ImportPreview } from '../../lib/types';

  const COLS: [string, string][] = [
    ['location', 'Locație'],
    ['address', 'Adresă'],
    ['lat', 'Lat.'],
    ['lon', 'Long.'],
    ['device', 'Echipament'],
    ['host', 'Host'],
    ['kind', 'Tip'],
    ['is_primary', 'Primar'],
    ['notes', 'Observații'],
  ];

  let file = $state<File | null>(null);
  let preview = $state<ImportPreview | null>(null);
  let error = $state('');
  let done = $state('');
  let busy = $state(false);

  function pick(e: Event) {
    file = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
    preview = null;
    error = done = '';
  }

  async function check() {
    if (!file) return;
    busy = true;
    error = done = '';
    try {
      preview = await upload<ImportPreview>('/api/admin/import/preview', file);
    } catch (e) {
      preview = null;
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function apply() {
    if (!file || !preview?.valid) return;
    busy = true;
    error = '';
    try {
      const r = await upload<{ locations: number; devices: number }>('/api/admin/import/apply', file);
      done = `S-au importat ${r.locations} ${r.locations === 1 ? 'locație' : 'locații'}.`;
      preview = null;
    } catch (e) {
      error = (e as Error).message;
      const p = e instanceof ApiError ? (e.data as { preview?: ImportPreview } | undefined)?.preview : undefined;
      if (p) preview = p;
    } finally {
      busy = false;
    }
  }

  function errorFor(row: ImportPreview['rows'][number], col: string) {
    return row.errors.filter((e) => e.column === col);
  }

  const sepName = (s: string) => (s === '\t' ? 'tab' : s);
</script>

<h1>Import / export</h1>

<section class="card box">
  <h2>Import locații</h2>
  <p class="muted">
    Fișier CSV (separator „,” „;” sau tab; UTF-8, Windows-1250 sau Windows-1252) sau Excel (.xlsx), maximum 5 MB, 1000 de rânduri și 200 de locații.
    Coloane: locație, adresă, latitudine, longitudine, echipament, host/IP, tip, primar, observații (obligatorii: locație, echipament, host).
    Locațiile existente nu sunt modificate.
  </p>
  <div class="row">
    <label class="file">
      Fișier CSV sau Excel
      <input type="file" accept=".csv,.txt,.xlsx" onchange={pick} />
    </label>
    <button type="button" onclick={check} disabled={!file || busy}>Previzualizează</button>
    <button type="button" class="primary" onclick={apply} disabled={!preview?.valid || busy}>Importă</button>
  </div>
  {#if error}<p class="notice danger" role="alert">{error}</p>{/if}
  {#if done}<p class="notice" role="status">{done} <a href="/administrare">Vezi lista de locații</a></p>{/if}

  {#if preview}
    <p class="summary" class:t-down={!preview.valid}>
      {preview.locations} locații, {preview.devices} echipamente, {preview.error_count} erori ·
      format {preview.format}{preview.encoding ? `, codificare ${preview.encoding}` : ''}{preview.separator ? `, separator ${sepName(preview.separator)}` : ''}
    </p>
    <div class="wrap">
      <table class="preview">
        <thead>
          <tr><th scope="col">Rând</th>{#each COLS as [, label] (label)}<th scope="col">{label}</th>{/each}</tr>
        </thead>
        <tbody>
          {#each preview.rows as r (r.line)}
            <tr class:bad={r.errors.length > 0}>
              <td>{r.line}</td>
              {#each COLS as [key] (key)}
                {@const errs = errorFor(r, key)}
                <td aria-invalid={errs.length > 0 ? 'true' : undefined}>
                  {r.values[key] ?? ''}
                  {#each errs as e (e.message)}<span class="error-text cell-err">{e.message}</span>{/each}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<section class="card box">
  <h2>Export locații</h2>
  <p class="muted">În formatul de import, reimportabil fără pierderi (fără pragurile proprii ale echipamentelor).</p>
  <div class="row">
    <a class="btn" href="/api/admin/export/locations.csv" download>Exportă locațiile (CSV)</a>
    <a class="btn" href="/api/admin/export/locations.xlsx" download>Exportă locațiile (Excel)</a>
  </div>
</section>

<style>
  .box {
    padding: 12px 16px;
    margin-bottom: 14px;
  }
  h2 {
    font-size: 1.05em;
    margin-top: 0;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: flex-end;
  }
  .file {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-weight: 600;
  }
  .btn {
    display: inline-block;
    padding: 5px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    text-decoration: none;
    color: var(--fg);
  }
  .summary {
    font-weight: 600;
  }
  .wrap {
    overflow-x: auto;
    max-height: 60vh;
  }
  .preview td {
    white-space: nowrap;
    font-size: 0.9em;
  }
  tr.bad td:first-child {
    color: var(--c-down);
    font-weight: 700;
  }
  td[aria-invalid='true'] {
    background: var(--danger-bg);
  }
  .cell-err {
    display: block;
    white-space: normal;
    max-width: 260px;
  }
</style>
