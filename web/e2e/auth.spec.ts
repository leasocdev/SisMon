import { expect, login, PASSWORD, test } from './fixtures';

test.describe('Administrare — autentificare', () => {
  test('parolă greșită, apoi corectă', async ({ page }) => {
    await page.goto('/administrare');
    await page.getByLabel('Parola', { exact: true }).fill('gresit-gresit');
    await page.getByRole('button', { name: 'Autentificare' }).click();
    await expect(page.getByRole('alert')).toContainText('Parolă incorectă');
    await page.getByLabel('Parola', { exact: true }).fill(PASSWORD);
    await page.getByRole('button', { name: 'Autentificare' }).click();
    await expect(page.getByRole('navigation', { name: 'Administrare' }).getByRole('link', { name: 'Locații' })).toBeVisible();
  });

  test('avertisment fără TLS', async ({ page }) => {
    await page.goto('/administrare');
    await expect(page.getByText('Conexiune necriptată')).toBeVisible();
  });

  test('ieșire', async ({ page }) => {
    await login(page);
    await page.getByRole('button', { name: 'Ieșire' }).click();
    await expect(page.getByLabel('Parola', { exact: true })).toBeVisible();
    const status = await page.evaluate(async () => (await fetch('/api/admin/locations')).status);
    expect(status).toBe(401);
  });

  test('sesiune expirată', async ({ page, context }) => {
    await login(page);
    await expect(page.locator('.loc-list li').first()).toBeVisible(); // no admin request in flight
    await context.clearCookies();
    await page.getByRole('navigation', { name: 'Administrare' }).getByRole('link', { name: 'Setări monitorizare' }).click();
    await expect(page.getByText('Sesiunea a expirat.')).toBeVisible();
    await expect(page.getByLabel('Parola', { exact: true })).toBeVisible();
  });

  test('prima configurare (server fără parolă)', async ({ page }) => {
    const base = 'http://127.0.0.1:18511';
    await page.goto(base + '/administrare');
    await expect(page.getByRole('heading', { name: 'Configurare inițială' })).toBeVisible();
    await page.getByLabel('Parola nouă').fill('abcdefghijkl1');
    await page.getByLabel('Repetați parola').fill('abcdefghijkl2');
    await page.getByRole('button', { name: 'Setează parola' }).click();
    await expect(page.getByText('Parolele nu coincid.')).toBeVisible();
    await page.getByLabel('Parola nouă').fill('scurt');
    await page.getByLabel('Repetați parola').fill('scurt');
    await page.getByRole('button', { name: 'Setează parola' }).click();
    await expect(page.getByText('Parola trebuie să aibă între 12 și 200 de caractere.')).toBeVisible();
    await page.getByLabel('Parola nouă').fill('parola-setup-123');
    await page.getByLabel('Repetați parola').fill('parola-setup-123');
    await page.getByRole('button', { name: 'Setează parola' }).click();
    await expect(page.getByRole('navigation', { name: 'Administrare' })).toBeVisible();
  });
});
