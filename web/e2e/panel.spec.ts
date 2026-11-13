import { adminApi, createLocation, expect, locationId, login, setHost, test, waitState } from './fixtures';

test.describe('Panou de detalii', () => {
  test('deschidere din card și din URL', async ({ page }) => {
    await page.goto('/');
    await page.locator('.loc-card', { hasText: 'Cahul' }).click();
    await expect(page).toHaveURL(/\/locatie\/\d+$/);
    const dialog = page.getByRole('dialog', { name: 'Cahul' });
    await expect(dialog).toContainText('Router');
    await expect(dialog).toContainText('10.0.1.1');
    await page.reload();
    await expect(page.getByRole('dialog', { name: 'Cahul' })).toContainText('10.0.1.1');
  });

  test('actualizare live și incident încheiat', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto(`/locatie/${id}`);
    const dialog = page.getByRole('dialog', { name: 'Cahul' });
    await expect(dialog.locator('.panel-state')).toContainText('✓ Funcțională');
    await setHost('10.0.1.1', 'timeout');
    await expect(dialog.locator('.panel-state')).toContainText('✕ Indisponibilă', { timeout: 20_000 });
    await setHost('10.0.1.1', 'ok');
    await expect(dialog.locator('.panel-state')).toContainText('✓ Funcțională', { timeout: 20_000 });
    await expect(dialog.locator('.incidents')).toContainText('Revenire', { timeout: 20_000 });
  });

  test('perioada 7 zile', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto(`/locatie/${id}`);
    const dialog = page.getByRole('dialog', { name: 'Cahul' });
    await expect(dialog).toContainText('Router');
    const req = page.waitForRequest((r) => r.url().includes(`/api/locations/${id}/timeline`) && (() => {
      const u = new URL(r.url());
      return Number(u.searchParams.get('to')) - Number(u.searchParams.get('from')) === 7 * 86_400_000;
    })());
    await dialog.getByRole('button', { name: '7 z' }).click();
    await req;
    await expect(dialog.getByRole('button', { name: '7 z' })).toHaveAttribute('aria-pressed', 'true');
  });

  test('grafic de latență', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto(`/locatie/${id}`);
    await expect(page.getByRole('dialog', { name: 'Cahul' }).locator('.uplot canvas')).toBeAttached();
  });

  test('Esc închide și readuce focusul pe card', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓');
    await page.locator('.loc-card', { hasText: 'Cahul' }).click();
    await expect(page.getByRole('dialog', { name: 'Cahul' })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.locator('.loc-card', { hasText: 'Cahul' })).toBeFocused();
  });

  test('locație inexistentă', async ({ page }) => {
    await page.goto('/locatie/999999');
    await expect(page.getByRole('dialog')).toContainText('Locația nu există sau a fost arhivată.');
  });
});

test('perioade alese rapid: se aplică ultima', async ({ page }) => {
  const id = await locationId('Cahul');
  await page.goto(`/locatie/${id}`);
  const dialog = page.getByRole('dialog', { name: 'Cahul' });
  await expect(dialog).toContainText('Router');
  await page.route(`**/api/locations/${id}/timeline*`, async (route) => {
    const u = new URL(route.request().url());
    if (Number(u.searchParams.get('to')) - Number(u.searchParams.get('from')) > 2 * 86_400_000) await new Promise((r) => setTimeout(r, 1500));
    await route.continue();
  });
  await dialog.getByRole('button', { name: '30 z' }).click();
  await dialog.getByRole('button', { name: '24 h' }).click();
  await page.waitForTimeout(2500);
  const from = await dialog.locator('.axis span').first().innerText();
  const [d, m, y] = from.split(' ')[0].split('.').map(Number);
  expect(Date.now() - new Date(y, m - 1, d).getTime()).toBeLessThan(3 * 86_400_000);
});

test('focusul rămâne în panou', async ({ page }) => {
  const id = await locationId('Cahul');
  await page.goto(`/locatie/${id}`);
  const dialog = page.getByRole('dialog', { name: 'Cahul' });
  await expect(dialog).toContainText('Router');
  for (let i = 0; i < 30; i++) {
    await page.keyboard.press(i % 7 === 6 ? 'Shift+Tab' : 'Tab');
    expect(await dialog.evaluate((el) => el.contains(document.activeElement)), `Tab ${i}`).toBe(true);
  }
});

test('rezumatul perioadei, perioadele de indisponibilitate și durata la echipamente', async ({ page }) => {
  const id = await locationId('Cahul');
  await page.goto(`/locatie/${id}`);
  const dialog = page.getByRole('dialog', { name: 'Cahul' });
  await expect(dialog.locator('.dsince').first()).toHaveText(/^de \d+/);
  await setHost('10.0.1.1', 'timeout');
  await expect(dialog.locator('.panel-state')).toContainText('✕ Indisponibilă', { timeout: 20_000 });
  await setHost('10.0.1.1', 'ok');
  await expect(dialog.locator('.panel-state')).toContainText('✓ Funcțională', { timeout: 20_000 });
  const row = dialog.locator('.outages tbody tr').first();
  await expect(row).toContainText('Indisponibilă', { timeout: 20_000 });
  await expect(row).toContainText('Router');
  await expect(dialog.locator('.psum')).toContainText(/Disponibilitate \d{1,3},\d{2}% · \d+ căder/);
  await expect(dialog.locator('.psum')).toContainText('indisponibilă');
});

test('legături: Statistici pentru toți, Editează doar pentru administrator', async ({ page }) => {
  const id = await locationId('Cahul');
  await page.goto(`/locatie/${id}`);
  const dialog = page.getByRole('dialog', { name: 'Cahul' });
  await expect(dialog.getByRole('link', { name: 'Statistici' })).toHaveAttribute('href', `/statistici?location=${id}&preset=24h`);
  await expect(dialog.getByRole('link', { name: 'Editează' })).toHaveCount(0);
  await dialog.getByRole('button', { name: '7 z' }).click();
  await expect(dialog.getByRole('link', { name: 'Statistici' })).toHaveAttribute('href', `/statistici?location=${id}&preset=7z`);
  await login(page);
  await page.goto(`/locatie/${id}`);
  await page.getByRole('dialog', { name: 'Cahul' }).getByRole('link', { name: 'Editează' }).click();
  await expect(page).toHaveURL(new RegExp(`/administrare/locatie/${id}$`));
});

test('echipamentele dezactivate sunt menționate', async ({ page }) => {
  const api = await adminApi();
  const id = await createLocation(api, { name: 'Dezactivat E2E', devices: [{ name: 'R', host: '10.0.9.1' }] });
  try {
    const until = Date.now() + 3 * 3_600_000;
    await api.send('PUT', `/api/admin/locations/${id}`, {
      name: 'Dezactivat E2E', address: '', notes: '', lat: null, lon: null,
      devices: [
        { name: 'R', host: '10.0.9.1', kind: '', is_primary: true, overrides: {} },
        { name: 'AP oprit', host: '10.0.9.2', kind: '', is_primary: false, overrides: {}, disabled: true, disabled_until: until },
      ],
    });
    await page.goto(`/locatie/${id}`);
    const box = page.getByRole('dialog', { name: 'Dezactivat E2E' }).locator('.disabled-devs');
    await expect(box).toContainText('dezactivat temporar');
    await expect(box).toContainText('AP oprit');
    await expect(box).toContainText('10.0.9.2');
    await expect(box).toContainText('până la');
  } finally {
    await api.send('DELETE', `/api/admin/locations/${id}`);
    await api.dispose();
  }
});

test('o cerere eșuată nu ascunde restul datelor', async ({ page }) => {
  const id = await locationId('Cahul');
  await page.route('**/api/devices/*/timeline*', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { message: 'Eroare de test' } }) }),
  );
  await page.goto(`/locatie/${id}`);
  const dialog = page.getByRole('dialog', { name: 'Cahul' });
  await expect(dialog.getByRole('alert')).toContainText('cronologia echipamentului Router');
  await expect(dialog.locator('.psum')).toContainText('Disponibilitate');
  await expect(dialog.locator('.axis span').first()).not.toHaveText('');
});
