import AxeBuilder from '@axe-core/playwright';
import { expect, locationId, login, setHost, test, waitState, type AdminApi } from './fixtures';
import type { Page } from '@playwright/test';

async function axe(page: Page, label: string) {
  const r = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).exclude('.leaflet-tile-pane').analyze();
  const found = r.violations.map((v) => `${label}: ${v.id} (${v.impact}) — ${v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')}`);
  expect(found, 'încălcări WCAG').toEqual([]);
}

for (const theme of ['light', 'dark'] as const) {
  test.describe(`accesibilitate — tema ${theme}`, () => {
    test.beforeEach(async ({ page }) => {
      await page.addInitScript((t) => localStorage.setItem('nw-theme', t), theme);
    });

    test('pagini publice', async ({ page }) => {
      await page.goto('/');
      await setHost('10.0.1.1', 'timeout');
      await setHost('10.0.2.2', 'timeout');
      await setHost('10.0.0.1', 'permission');
      await waitState(page, 'Cahul', '✕ Indisponibilă');
      await waitState(page, 'Bălți', '▲ Degradată');
      await axe(page, '/');
      const id = await locationId('Cahul');
      await page.goto(`/locatie/${id}`);
      await expect(page.getByRole('dialog', { name: 'Cahul' }).locator('.uplot')).toBeAttached();
      await axe(page, '/locatie');
      for (const path of ['/dispecerat', '/harta', '/statistici', '/raport-24h']) {
        await page.goto(path);
        await page.waitForLoadState('networkidle');
        await axe(page, path);
      }
    });

    test('administrare', async ({ page }) => {
      await page.goto('/administrare');
      await expect(page.getByLabel('Parola', { exact: true })).toBeVisible();
      await axe(page, 'login');
      await login(page);
      await axe(page, '/administrare');
      for (const p of ['setari', 'identitate', 'backup', 'audit', 'diagnostic', 'parola', 'despre']) {
        await page.goto(`/administrare/${p}`);
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
        await axe(page, p);
      }
      await page.goto(`/administrare/locatie/${await locationId('Cahul')}`);
      await expect(page.locator('.device-row').first()).toBeVisible();
      await axe(page, 'editor');
    });
  });
}

test('navigare din tastatură cu focus vizibil', async ({ page }) => {
  await page.goto('/');
  await waitState(page, 'Cahul', '✓');
  const seen = new Set<string>();
  for (let i = 0; i < 25; i++) {
    await page.keyboard.press('Tab');
    const info = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null;
      if (!el || el === document.body) return null;
      const cs = getComputedStyle(el);
      return { tag: el.tagName, cls: el.className, outline: cs.outlineStyle, width: cs.outlineWidth };
    });
    if (!info && i > 5) break; // wrapped past the last element
    expect(info, `pasul ${i}`).not.toBeNull();
    expect(info!.outline, `focus vizibil pe ${info!.tag}.${info!.cls}`).not.toBe('none');
    seen.add(info!.tag + (String(info!.cls).includes('loc-card') ? '.card' : String(info!.cls).includes('tot') ? '.filter' : ''));
  }
  expect([...seen]).toEqual(expect.arrayContaining(['A', 'BUTTON', 'BUTTON.filter', 'INPUT', 'A.card']));
});

export type { AdminApi };
