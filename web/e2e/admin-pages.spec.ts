import { expect, login, PASSWORD, test } from './fixtures';

test.describe('Administrare — pagini', () => {
  test('setări: eroare pe câmp, apoi salvare', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/setari');
    const interval = page.getByLabel('Interval între verificări (s)');
    await expect(interval).toHaveValue('60');
    await interval.fill('5');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(interval).toHaveAttribute('aria-invalid', 'true');
    await expect(page.locator('.field', { has: interval }).locator('.error-text')).toContainText('Intervalul');
    await interval.fill('30');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Setări salvate.' })).toBeVisible();
    await page.reload();
    await expect(page.getByLabel('Interval între verificări (s)')).toHaveValue('30');
    await page.getByLabel('Interval între verificări (s)').fill('60');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Setări salvate.' })).toBeVisible();
  });

  test('identitate', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/identitate');
    await page.getByLabel('Denumirea aplicației').fill('Dispecerat Test');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.locator('.hdr .name')).toHaveText('Dispecerat Test');
    await page.getByLabel('Denumirea aplicației').fill('SisMon');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.locator('.hdr .name')).toHaveText('SisMon');
  });

  test('backup manual', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/backup');
    const rows = page.locator('table.backups tbody tr');
    const before = await rows.count();
    await page.getByRole('button', { name: 'Creează backup acum' }).click();
    await expect(rows).toHaveCount(before + 1);
    const first = rows.first();
    await expect(first).toContainText('manual');
    const href = await first.getByRole('link', { name: 'Descarcă' }).getAttribute('href');
    expect(href).toMatch(/^\/api\/admin\/backups\/.+\.db$/);
    await expect(page.getByText('sismon restore')).toBeVisible();
  });

  test('jurnal audit', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/setari');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Setări salvate.' })).toBeVisible();
    await page.goto('/administrare/audit');
    await expect(page.locator('table.audit')).toContainText('Setări modificate');
  });

  test('diagnostic', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/diagnostic');
    await expect(page.locator('.diag')).toContainText('Simulat');
    await expect(page.locator('.diag')).toContainText('1.7.0');
  });

  test('despre', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/despre');
    await expect(page.getByText('OpenStreetMap')).toBeVisible();
  });

  test('schimbarea parolei', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/parola');
    await page.getByLabel('Parola actuală').fill('gresita-gresita');
    await page.getByLabel('Parola nouă', { exact: true }).fill('parola-e2e-67890');
    await page.getByLabel('Repetați parola nouă').fill('parola-e2e-67890');
    await page.getByRole('button', { name: 'Schimbă parola' }).click();
    await expect(page.getByLabel('Parola actuală')).toHaveAttribute('aria-invalid', 'true');
    await expect(page.getByText('Parola actuală este incorectă.')).toBeVisible();
    await page.getByLabel('Parola actuală').fill(PASSWORD);
    await page.getByRole('button', { name: 'Schimbă parola' }).click();
    await expect(page.getByText('Parola a fost schimbată. Autentificați-vă din nou.')).toBeVisible();
    await page.getByLabel('Parola', { exact: true }).fill('parola-e2e-67890');
    await page.getByRole('button', { name: 'Autentificare' }).click();
    await expect(page.getByRole('navigation', { name: 'Administrare' })).toBeVisible();
    // restore the shared password
    await page.goto('/administrare/parola');
    await page.getByLabel('Parola actuală').fill('parola-e2e-67890');
    await page.getByLabel('Parola nouă', { exact: true }).fill(PASSWORD);
    await page.getByLabel('Repetați parola nouă').fill(PASSWORD);
    await page.getByRole('button', { name: 'Schimbă parola' }).click();
    await expect(page.getByText('Parola a fost schimbată.')).toBeVisible();
  });
});
