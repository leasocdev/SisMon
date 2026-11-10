import { adminApi, control, createLocation, expect, setHost, test } from './fixtures';

test.describe('Dispecerat', () => {
  test.use({ viewport: { width: 1920, height: 1080 } });

  test('bandă cu problemele curente', async ({ page }) => {
    await page.goto('/dispecerat');
    await expect(page.locator('.tile', { hasText: 'Cahul' })).toContainText('Funcțională');
    await setHost('10.0.1.1', 'timeout');
    await expect(page.locator('.band')).toContainText('Cahul · Router · de', { timeout: 20_000 });
  });

  test.describe('conexiune', () => {
    test.use({ allowConsole: [/EventSource/] });
    test('pierderea conexiunii afișează DATE NEACTUALIZATE', async ({ page }) => {
      await page.clock.install();
      await page.goto('/dispecerat');
      await expect(page.getByRole('status').filter({ hasText: 'Conectat' })).toBeVisible();
      await control('/sse', { on: false });
      await page.clock.fastForward(31_000);
      await expect(page.getByText('DATE NEACTUALIZATE', { exact: true })).toBeVisible();
      await control('/sse', { on: true });
      await page.clock.fastForward(35_000);
      await expect(page.getByText('DATE NEACTUALIZATE', { exact: true })).toHaveCount(0, { timeout: 20_000 });
    });
  });

  test('60 de locații încap fără derulare', async ({ page }) => {
    const api = await adminApi();
    const ids: number[] = [];
    try {
      for (let i = 0; i < 57; i++) {
        ids.push(await createLocation(api, { name: `Test dispecerat ${String(i + 1).padStart(2, '0')}`, devices: [{ name: 'Router', host: `10.1.${i}.1` }] }));
      }
      await page.goto('/dispecerat');
      await expect(page.locator('.tile')).toHaveCount(60);
      const m = await page.evaluate(() => ({
        sh: document.documentElement.scrollHeight,
        sw: document.documentElement.scrollWidth,
        ih: innerHeight,
        iw: innerWidth,
      }));
      expect(m.sh).toBeLessThanOrEqual(m.ih);
      expect(m.sw).toBeLessThanOrEqual(m.iw);
      for (const t of await page.locator('.tile').all()) await expect(t).toBeInViewport({ ratio: 1 });
    } finally {
      for (const id of ids) await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test('temă întunecată implicit, luminoasă din URL', async ({ page }) => {
    await page.goto('/dispecerat');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await page.goto('/dispecerat?tema=luminoasa');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });

  test('fără meniu, cu ceas', async ({ page }) => {
    await page.goto('/dispecerat');
    await expect(page.getByRole('navigation', { name: 'Navigare principală' })).toHaveCount(0);
    await expect(page.locator('.clock')).toHaveText(/^\d{2}:\d{2}:\d{2}$/);
  });
});
