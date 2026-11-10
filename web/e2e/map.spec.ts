import { adminApi, control, createLocation, expect, locationId, setHost, test } from './fixtures';

test.describe('Hartă', () => {
  test('marcaje colorate după stare', async ({ page }) => {
    await page.goto('/harta');
    await expect(page.locator('.leaflet-container')).toBeVisible();
    await expect(page.locator('.nw-marker')).toHaveCount(3, { timeout: 20_000 });
    await setHost('10.0.1.1', 'timeout');
    await expect(page.locator('.nw-marker.state-down')).toHaveCount(1, { timeout: 20_000 });
  });

  test('popup cu legătură spre panou', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto('/harta');
    await page.locator('.nw-marker[data-id="' + id + '"]').click();
    const popup = page.locator('.leaflet-popup');
    await expect(popup).toContainText('Cahul');
    await popup.getByRole('link', { name: 'Detalii' }).click();
    await expect(page).toHaveURL(new RegExp(`/locatie/${id}$`));
  });

  test('popup cu nume HTML afișat literal', async ({ page }) => {
    const api = await adminApi();
    const name = '<img src=x onerror=window.__xss=1>Ștefan Vodă';
    const id = await createLocation(api, { name, lat: 46.51, lon: 29.66, devices: [{ name: 'R', host: '10.0.8.1' }] });
    try {
      await page.goto('/harta');
      await page.locator('.nw-marker[data-id="' + id + '"]').click();
      await expect(page.locator('.leaflet-popup')).toContainText(name);
      expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();
    } finally {
      await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test('locațiile fără coordonate sunt listate', async ({ page }) => {
    const api = await adminApi();
    const id = await createLocation(api, { name: 'Fără GPS', devices: [{ name: 'R', host: '10.0.8.2' }] });
    try {
      await page.goto('/harta');
      await expect(page.locator('.no-coords')).toContainText('Fără GPS');
    } finally {
      await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test.describe('rezervă', () => {
    test.use({ allowConsole: [/tiles|dale/i] });
    test('contururi locale când dalele nu se încarcă, revenire automată', async ({ page }) => {
      await page.clock.install();
      await control('/tiles', { ok: false });
      await page.goto('/harta');
      await expect(page.locator('path.contour').first()).toBeAttached({ timeout: 15_000 });
      await expect(page.locator('.fallback-note')).toBeVisible();
      await control('/tiles', { ok: true });
      await page.clock.fastForward(61_000);
      await expect(page.locator('path.contour')).toHaveCount(0, { timeout: 15_000 });
      await expect(page.locator('.leaflet-tile-loaded').first()).toBeAttached();
    });
  });
});
