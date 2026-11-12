<script lang="ts">
  import { ApiError, send } from '../lib/api';
  import { fmtLatency } from '../lib/status';
  import { fmtDateTime, utcToZonedParts, zonedToUtc } from '../lib/time';
  import type { DeviceInput, Settings, TestResult } from '../lib/types';

  let {
    device = $bindable(),
    index,
    count,
    errors,
    defaults,
    onMove,
    onRemove,
    onDragStart,
    onDrop,
  }: {
    device: DeviceInput;
    index: number;
    count: number;
    errors: Record<string, string>;
    defaults: Settings | null;
    onMove: (to: number) => void;
    onRemove: () => void;
    onDragStart: () => void;
    onDrop: () => void;
  } = $props();

  const KINDS = ['PC', 'Router', 'Switch', 'Controler', 'Video', 'Receptor', 'Server', 'Altele'];

  let testing = $state(false);
  let result = $state<{ ok: boolean; text: string } | null>(null);
  let showOverrides = $state(Object.keys(device.overrides).length > 0);

  // Data reactivării, editată în ora Chișinăului.
  const initial = device.disabled_until != null ? utcToZonedParts(device.disabled_until) : null;
  let untilDate = $state(initial?.date ?? '');
  let untilTime = $state(initial?.time ?? '');

  function setUntil() {
    if (!untilDate) {
      device.disabled_until = null;
      return;
    }
    const ms = zonedToUtc(untilDate, untilTime || '00:00');
    device.disabled_until = Number.isNaN(ms) ? null : ms;
  }

  function preset(days: number | null) {
    if (days === null) {
      untilDate = untilTime = '';
    } else {
      const p = utcToZonedParts(Date.now() + days * 86_400_000);
      untilDate = p.date;
      untilTime = p.time;
    }
    setUntil();
  }

  function toggleDisabled(on: boolean) {
    device.disabled = on;
    if (!on) preset(null);
  }

  const n = $derived(index + 1);
  const id = (f: string) => `dev-${index}-${f}`;

  async function test() {
    testing = true;
    result = null;
    try {
      const r = await send<TestResult>('POST', '/api/admin/devices/test', { host: device.host.trim() });
      result = r.ok ? { ok: true, text: `✓ Răspunde · ${fmtLatency(r.latency_ms)}` } : { ok: false, text: '✕ ' + r.message };
    } catch (e) {
      result = { ok: false, text: e instanceof ApiError ? e.message : 'Testul a eșuat.' };
    } finally {
      testing = false;
    }
  }

  function setOverride(k: 'interval_s' | 'failures' | 'recoveries', v: string) {
    const o = { ...device.overrides };
    if (v.trim() === '') delete o[k];
    else o[k] = Number(v);
    device.overrides = o;
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="device-row card" class:off={device.disabled} ondragover={(e) => e.preventDefault()} ondrop={(e) => (e.preventDefault(), onDrop())}>
  <div class="handle" draggable="true" ondragstart={onDragStart} title="Trageți pentru a reordona" aria-hidden="true">⠿</div>
  <div class="fields">
    <div class="field">
      <label for={id('name')}>Denumire echipament <span class="sr-only">{n}</span></label>
      <input id={id('name')} bind:value={device.name} aria-invalid={!!errors.name} aria-describedby={errors.name ? id('name-err') : undefined} />
      {#if errors.name}<p class="error-text" id={id('name-err')}>{errors.name}</p>{/if}
    </div>
    <div class="field">
      <label for={id('host')}>Adresă IP / DNS <span class="sr-only">{n}</span></label>
      <input id={id('host')} class="mono" bind:value={device.host} aria-invalid={!!errors.host} aria-describedby={errors.host ? id('host-err') : undefined} />
      {#if errors.host}<p class="error-text" id={id('host-err')}>{errors.host}</p>{/if}
    </div>
    <div class="field">
      <label for={id('kind')}>Tip <span class="sr-only">{n}</span></label>
      <select id={id('kind')} value={device.kind || 'Altele'} onchange={(e) => (device.kind = e.currentTarget.value)} aria-invalid={!!errors.kind}>
        {#each KINDS as k (k)}<option value={k}>{k}</option>{/each}
        {#if device.kind && !KINDS.includes(device.kind)}<option value={device.kind}>{device.kind}</option>{/if}
      </select>
      {#if errors.kind}<p class="error-text">{errors.kind}</p>{/if}
    </div>
    <label class="primary-box"><input type="checkbox" bind:checked={device.is_primary} /> ★ Primar <span class="sr-only">{n}</span></label>
  </div>
  <div class="actions">
    <button type="button" onclick={test} disabled={testing || !device.host.trim()}>{testing ? 'Se testează…' : 'Testează acum'}</button>
    {#if result}<span class="test-result" class:t-up={result.ok} class:t-down={!result.ok} role="status">{result.text}</span>{/if}
    <span class="spacer"></span>
    <label class="disable-box">
      <input type="checkbox" checked={device.disabled} onchange={(e) => toggleDisabled(e.currentTarget.checked)} />
      Dezactivat <span class="sr-only">{n}</span>
    </label>
    <button type="button" onclick={() => (showOverrides = !showOverrides)} aria-expanded={showOverrides}>Praguri proprii</button>
    <button type="button" onclick={() => onMove(index - 1)} disabled={index === 0} aria-label="Mută în sus">↑</button>
    <button type="button" onclick={() => onMove(index + 1)} disabled={index === count - 1} aria-label="Mută în jos">↓</button>
    <button type="button" onclick={onRemove} disabled={count === 1} aria-label="Șterge rândul {n}">Șterge</button>
  </div>
  {#if device.disabled}
    <div class="disabled-panel" role="group" aria-label="Dezactivare echipament {n}">
      <p class="what">Nu se monitorizează și nu apare pe Monitorizare, hartă sau dispecerat. Perioada nu intră în statistici.</p>
      <div class="until">
        <div class="field">
          <label for={id('ud')}>Reactivare automată — data</label>
          <input id={id('ud')} type="date" bind:value={untilDate} oninput={setUntil} aria-invalid={!!errors.disabled_until} />
        </div>
        <div class="field">
          <label for={id('ut')}>ora</label>
          <input id={id('ut')} type="time" bind:value={untilTime} oninput={setUntil} disabled={!untilDate} />
        </div>
        <div class="presets">
          <button type="button" onclick={() => preset(1)}>+1 zi</button>
          <button type="button" onclick={() => preset(7)}>+7 zile</button>
          <button type="button" onclick={() => preset(null)} disabled={!untilDate}>Fără dată</button>
        </div>
      </div>
      {#if errors.disabled_until}<p class="error-text">{errors.disabled_until}</p>{/if}
      <p class="hint muted" role="status">
        {#if device.disabled_until != null}Se reactivează automat la {fmtDateTime(device.disabled_until)}.
        {:else}Rămâne dezactivat până îl reactivați manual.{/if}
      </p>
    </div>
  {/if}
  {#if showOverrides}
    <div class="overrides">
      <div class="field">
        <label for={id('iv')}>Interval (s)</label>
        <input id={id('iv')} type="number" min="10" max="3600" value={device.overrides.interval_s ?? ''} placeholder={String(defaults?.interval_s ?? '')}
          oninput={(e) => setOverride('interval_s', e.currentTarget.value)} aria-invalid={!!errors.interval_s} />
        {#if errors.interval_s}<p class="error-text">{errors.interval_s}</p>{/if}
      </div>
      <div class="field">
        <label for={id('fl')}>Eșecuri pentru cădere</label>
        <input id={id('fl')} type="number" min="1" max="20" value={device.overrides.failures ?? ''} placeholder={String(defaults?.failures ?? '')}
          oninput={(e) => setOverride('failures', e.currentTarget.value)} aria-invalid={!!errors.failures} />
        {#if errors.failures}<p class="error-text">{errors.failures}</p>{/if}
      </div>
      <div class="field">
        <label for={id('rc')}>Reușite pentru revenire</label>
        <input id={id('rc')} type="number" min="1" max="20" value={device.overrides.recoveries ?? ''} placeholder={String(defaults?.recoveries ?? '')}
          oninput={(e) => setOverride('recoveries', e.currentTarget.value)} aria-invalid={!!errors.recoveries} />
        {#if errors.recoveries}<p class="error-text">{errors.recoveries}</p>{/if}
      </div>
      <p class="muted hint">Lăsați gol pentru valorile globale (afișate ca exemplu).</p>
    </div>
  {/if}
  {#if errors.stale_s || errors.id}<p class="error-text">{errors.stale_s || errors.id}</p>{/if}
</div>

<style>
  .device-row {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: 4px 10px;
    padding: 10px;
  }
  .handle {
    grid-row: span 3;
    cursor: grab;
    color: var(--muted);
    user-select: none;
    font-size: 1.2em;
  }
  .fields {
    display: grid;
    grid-template-columns: minmax(140px, 1.2fr) minmax(140px, 1fr) minmax(110px, 0.8fr) auto;
    gap: 8px;
    align-items: start;
  }
  @media (max-width: 760px) {
    .fields {
      grid-template-columns: 1fr;
    }
  }
  .fields .field {
    margin: 0;
  }
  .fields select {
    width: 100%;
  }
  .primary-box {
    display: flex;
    gap: 6px;
    align-items: center;
    margin-top: 24px;
    white-space: nowrap;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  }
  .spacer {
    flex: 1;
  }
  .test-result {
    font-weight: 600;
  }
  .device-row.off {
    border-style: dashed;
    background: color-mix(in srgb, var(--muted) 10%, var(--surface));
  }
  .device-row.off .fields {
    opacity: 0.7;
  }
  .disable-box {
    display: inline-flex;
    gap: 6px;
    align-items: center;
    white-space: nowrap;
    font-weight: 600;
  }
  .disabled-panel {
    border-left: 3px solid var(--muted);
    padding: 4px 0 4px 10px;
  }
  .disabled-panel .what {
    margin: 0 0 6px;
    font-size: 0.9em;
  }
  .until {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    align-items: flex-end;
  }
  .until .field {
    margin: 0;
  }
  .presets {
    display: flex;
    gap: 6px;
  }
  .overrides {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    align-items: flex-end;
  }
  .overrides input {
    width: 120px;
  }
  .hint {
    font-size: 0.85em;
    margin: 0 0 12px;
  }
  .mono {
    font-family: var(--mono);
  }
</style>
