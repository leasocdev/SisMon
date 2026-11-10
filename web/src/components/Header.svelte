<script lang="ts">
  import type { Snippet } from 'svelte';
  import { router } from '../lib/router.svelte';
  import type { Theme } from '../lib/theme';

  let {
    name,
    subtitle = '',
    logoVersion = 0,
    theme,
    onTheme,
    status,
  }: { name: string; subtitle?: string; logoVersion?: number; theme: Theme; onTheme: (t: Theme) => void; status?: Snippet } = $props();

  const links = [
    { href: '/', label: 'Monitorizare', match: ['monitor', 'location'] },
    { href: '/harta', label: 'Hartă', match: ['map'] },
    { href: '/statistici', label: 'Statistici', match: ['stats'] },
    { href: '/raport-24h', label: 'Raport 24H', match: ['report'] },
    { href: '/dispecerat', label: 'Dispecerat', match: ['dispatch'] },
    { href: '/administrare', label: 'Administrare', match: ['admin', 'adminLocation', 'adminPage'] },
  ];
</script>

<header class="hdr">
  <a class="brand" href="/">
    {#if logoVersion > 0}<img class="logo" src="/branding/logo?v={logoVersion}" alt="" />{/if}
    <span class="texts">
    <span class="name">{name}</span>
    {#if subtitle}<span class="sub">{subtitle}</span>{/if}
    </span>
  </a>
  <nav aria-label="Navigare principală">
    <ul>
      {#each links as l (l.href)}
        <li>
          <a href={l.href} aria-current={l.match.includes(router.route.name) ? 'page' : undefined}>{l.label}</a>
        </li>
      {/each}
    </ul>
  </nav>
  <div class="right">
    {@render status?.()}
    <button
      type="button"
      class="theme"
      onclick={() => onTheme(theme === 'dark' ? 'light' : 'dark')}
      aria-label={theme === 'dark' ? 'Comută la tema luminoasă' : 'Comută la tema întunecată'}
    >
      {theme === 'dark' ? '☀ Luminoasă' : '☾ Întunecată'}
    </button>
  </div>
</header>

<style>
  .hdr {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 8px 16px;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    flex-wrap: wrap;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    text-decoration: none;
    color: var(--fg);
    line-height: 1.1;
  }
  .texts {
    display: flex;
    flex-direction: column;
  }
  .logo {
    height: 32px;
    max-width: 120px;
    object-fit: contain;
  }
  .name {
    font-weight: 700;
    font-size: 1.15em;
  }
  .sub {
    font-size: 0.72em;
    letter-spacing: 0.08em;
    color: var(--muted);
  }
  nav ul {
    display: flex;
    gap: 4px;
    list-style: none;
    margin: 0;
    padding: 0;
    flex-wrap: wrap;
  }
  nav a {
    display: block;
    padding: 6px 10px;
    border-radius: var(--radius);
    color: var(--fg);
    text-decoration: none;
  }
  nav a:hover {
    background: var(--surface-2);
  }
  nav a[aria-current='page'] {
    background: var(--surface-2);
    font-weight: 600;
    box-shadow: inset 0 -2px 0 var(--focus);
  }
  .right {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 8px;
  }
</style>
