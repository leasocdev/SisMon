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

  test('popup cu durata stării, cronologie 24 h și actualizare în timp real', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto('/harta');
    await page.locator('.nw-marker[data-id="' + id + '"]').click();
    const popup = page.locator('.leaflet-popup');
    await expect(popup.locator('.pstate')).toContainText('Funcțională');
    await expect(popup.locator('.pstate')).toContainText(/Funcțională de \d+ (s|min) \(de la \d\d:\d\d\)/);
    await expect(popup.locator('.mini-timeline')).toBeVisible();
    // the open popup follows the live state, without reopening it
    await setHost('10.0.1.1', 'timeout');
    await expect(popup.locator('.pstate')).toContainText('Indisponibilă', { timeout: 20_000 });
    await expect(popup).toContainText('Router (10.0.1.1)');
  });

  test('indiciu la trecerea mouse-ului peste marcaj', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto('/harta');
    await page.locator('.nw-marker[data-id="' + id + '"]').hover();
    const hint = page.locator('.leaflet-tooltip.nw-hint');
    await expect(hint).toBeVisible();
    await expect(hint).toHaveText(/^Cahul · \S+ (Funcțională|Indisponibilă|Degradată|Fără date) · \d+ (s|min)/);
    // the marker keeps an accessible name, without a second native tooltip
    const marker = page.locator('.leaflet-marker-icon', { has: page.locator('.nw-marker[data-id="' + id + '"]') });
    await expect(marker).toHaveAttribute('aria-label', 'Cahul');
    await expect(marker).not.toHaveAttribute('title');
  });

  test('popup cu nume HTML afișat literal', async ({ page }) => {
    const api = await adminApi();
    const name = '<img src=x onerror=window.__xss=1>Ștefan Vodă';
    const id = await createLocation(api, { name, lat: 46.51, lon: 29.66, devices: [{ name: 'R', host: '10.0.8.1' }] });
    try {
      await page.goto('/harta');
      await page.locator('.nw-marker[data-id="' + id + '"]').click();
      await expect(page.locator('.leaflet-popup')).toContainText(name);
      // the hover hint shows the name as text too
      await page.locator('.nw-marker[data-id="' + id + '"]').hover();
      await expect(page.locator('.leaflet-tooltip.nw-hint')).toContainText(name);
      expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();
    } finally {
      await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test('locațiile fără coordonate nu sunt listate', async ({ page }) => {
    const api = await adminApi();
    const id = await createLocation(api, { name: 'Fără GPS', devices: [{ name: 'R', host: '10.0.8.2' }] });
    try {
      await page.goto('/harta');
      await expect(page.locator('.leaflet-container')).toBeVisible();
      await expect(page.locator('.no-coords')).toHaveCount(0);
    } finally {
      await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test('butoanele Centrează și Ecran complet, harta pe toată fereastra', async ({ page }) => {
    await page.goto('/harta');
    await expect(page.getByRole('button', { name: /Centrează/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Ecran complet/ })).toBeVisible();
    await page.getByRole('button', { name: /Centrează/ }).click();
    const box = await page.locator('.leaflet-container').boundingBox();
    const vp = page.viewportSize()!;
    expect(box!.x).toBe(0);
    expect(Math.round(box!.x + box!.width)).toBe(vp.width);
    expect(Math.round(box!.y + box!.height)).toBe(vp.height);
    await expect(page.locator('.nw-label[class*="pos-"]').first()).toBeVisible();
  });

  test.describe('rezervă', () => {
    test.use({ allowConsole: [/tiles|dale/i] });
    test('contururi locale când dalele nu se încarcă, revenire automată', async ({ page }) => {
      await page.clock.install();
      await control('/tiles', { ok: false });
      await page.goto('/harta');
      // 3 reîncercări eșalonate per dală (≈ 5–12 s) înainte de trecerea la contururi.
      await expect(page.locator('path.contour').first()).toBeAttached({ timeout: 25_000 });
      await expect(page.locator('.fallback-note')).toBeVisible();
      await control('/tiles', { ok: true });
      await page.clock.fastForward(61_000);
      await expect(page.locator('path.contour')).toHaveCount(0, { timeout: 15_000 });
      await expect(page.locator('.leaflet-tile-loaded').first()).toBeAttached();
    });
  });
});
