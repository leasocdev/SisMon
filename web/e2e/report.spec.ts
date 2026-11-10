import { adminApi, createLocation, expect, setHost, test, waitState } from './fixtures';

test.describe('Raport 24H', () => {
  test('situația curentă se actualizează live', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓ Funcțională');
    await page.goto('/raport-24h');
    await expect(page.locator('.summary')).toContainText('Afectate acum: 0');
    await setHost('10.0.1.1', 'timeout');
    await expect(page.locator('table.current')).toContainText('Cahul', { timeout: 25_000 });
    await expect(page.locator('.summary')).toContainText('Afectate acum: 1');
  });

  test('întreruperile ultimelor 24 h', async ({ page }) => {
    await page.goto('/raport-24h');
    await setHost('10.0.1.1', 'timeout');
    await expect(page.locator('table.current')).toContainText('Cahul', { timeout: 25_000 });
    await setHost('10.0.1.1', 'ok');
    await expect(page.locator('.outages')).toContainText('Revenire', { timeout: 25_000 });
  });

  test('titlul raportului și fusul orar', async ({ page }) => {
    await page.goto('/raport-24h');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Raport de disponibilitate');
    await expect(page.getByText('Ora: Europe/Chisinau')).toBeVisible();
  });

  test('stil de tipărire', async ({ page }) => {
    await page.goto('/raport-24h');
    await expect(page.locator('.summary')).toBeVisible();
    await page.emulateMedia({ media: 'print' });
    await expect(page.getByRole('navigation', { name: 'Navigare principală' })).toBeHidden();
    await expect(page.getByRole('button', { name: 'Tipărește' })).toBeHidden();
    await expect(page.locator('table.current')).toBeVisible();
  });
});

test('două echipamente primare cu același nume, ambele căzute', async ({ page }) => {
  const api = await adminApi();
  const id = await createLocation(api, {
    name: 'Dublură E2E',
    devices: [{ name: 'Router', host: '10.0.9.1', is_primary: true }, { name: 'Router', host: '10.0.9.2', is_primary: true }],
  });
  try {
    await setHost('10.0.9.1', 'timeout');
    await setHost('10.0.9.2', 'timeout');
    await page.goto('/raport-24h');
    await expect(page.locator('table.current tr', { hasText: 'Dublură E2E' })).toHaveCount(2, { timeout: 25_000 });
  } finally {
    await api.send('DELETE', `/api/admin/locations/${id}`);
    await api.dispose();
  }
});
