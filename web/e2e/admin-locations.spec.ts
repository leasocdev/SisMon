import { adminApi, createLocation, expect, locationId, login, PASSWORD, setHost, test } from './fixtures';

test.describe('Administrare — locații', () => {
  test('creare cu „Testează acum”', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Adaugă locație' }).click();
    await page.getByLabel('Denumire', { exact: true }).fill('Orhei E2E');
    const rows = page.locator('.device-row');
    await rows.nth(0).getByLabel('Denumire echipament').fill('Router');
    await rows.nth(0).getByLabel('Adresă IP / DNS').fill('10.0.5.1');
    await rows.nth(0).getByRole('button', { name: 'Testează acum' }).click();
    await expect(rows.nth(0).locator('.test-result')).toContainText('✓ Răspunde');
    await setHost('10.0.5.2', 'timeout');
    await page.getByRole('button', { name: 'Adaugă echipament' }).click();
    await rows.nth(1).getByLabel('Denumire echipament').fill('AP');
    await rows.nth(1).getByLabel('Adresă IP / DNS').fill('10.0.5.2');
    await rows.nth(1).getByRole('button', { name: 'Testează acum' }).click();
    await expect(rows.nth(1).locator('.test-result')).toContainText('Fără răspuns în timpul stabilit.');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Locația a fost salvată.' })).toBeVisible();
    await page.goto('/');
    await expect(page.locator('.loc-card', { hasText: 'Orhei E2E' })).toBeVisible({ timeout: 10_000 });
    const api = await adminApi();
    await api.send('DELETE', `/api/admin/locations/${await locationId('Orhei E2E')}`);
    await api.dispose();
  });

  test('eroare de validare pe câmpul host', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Adaugă locație' }).click();
    await page.getByLabel('Denumire', { exact: true }).fill('Greșit E2E');
    const row = page.locator('.device-row').nth(0);
    await row.getByLabel('Denumire echipament').fill('R');
    await row.getByLabel('Adresă IP / DNS').fill('999.1.1.1');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(row.getByLabel('Adresă IP / DNS')).toHaveAttribute('aria-invalid', 'true');
    await expect(row.locator('.error-text')).toContainText('Adresă IP sau nume DNS invalid');
    await expect(row.getByLabel('Adresă IP / DNS')).toBeFocused();
  });

  test('fără echipament primar', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Adaugă locație' }).click();
    await page.getByLabel('Denumire', { exact: true }).fill('Fără primar E2E');
    const row = page.locator('.device-row').nth(0);
    await row.getByLabel('Denumire echipament').fill('R');
    await row.getByLabel('Adresă IP / DNS').fill('10.0.5.9');
    await row.getByLabel('Primar').uncheck();
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByText('Bifați cel puțin un echipament primar.')).toBeVisible();
  });

  test('modificări nesalvate', async ({ page }) => {
    await login(page);
    await page.locator('.loc-list').getByRole('link', { name: 'Cahul' }).click();
    await page.getByLabel('Denumire', { exact: true }).fill('Cahul modificat');
    const url = page.url();
    await page.locator('header a.brand').click();
    const dlg = page.getByRole('alertdialog');
    await expect(dlg).toContainText('Aveți modificări nesalvate. Le abandonați?');
    await dlg.getByRole('button', { name: 'Rămân' }).click();
    expect(page.url()).toBe(url);
    await expect(page.getByLabel('Denumire', { exact: true })).toHaveValue('Cahul modificat');
    await page.locator('header a.brand').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Abandonează' }).click();
    await expect(page).toHaveURL(/\/$/);
  });

  test('reordonarea echipamentelor', async ({ page }) => {
    const api = await adminApi();
    const id = await createLocation(api, { name: 'Ordine E2E', devices: [{ name: 'Primul', host: '10.0.6.1' }, { name: 'Al doilea', host: '10.0.6.2' }] });
    try {
      await login(page);
      await page.goto(`/administrare/locatie/${id}`);
      const rows = page.locator('.device-row');
      await expect(rows.nth(0).getByLabel('Denumire echipament')).toHaveValue('Primul');
      await rows.nth(0).getByRole('button', { name: 'Mută în jos' }).click();
      await expect(rows.nth(0).getByLabel('Denumire echipament')).toHaveValue('Al doilea');
      await page.getByRole('button', { name: 'Salvează' }).click();
      await expect(page.getByRole('status').filter({ hasText: 'Locația a fost salvată.' })).toBeVisible();
      await page.reload();
      await expect(page.locator('.device-row').nth(0).getByLabel('Denumire echipament')).toHaveValue('Al doilea');
    } finally {
      await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test('arhivare', async ({ page }) => {
    const api = await adminApi();
    const id = await createLocation(api, { name: 'Arhivă E2E', devices: [{ name: 'R', host: '10.0.6.5' }] });
    await api.dispose();
    await login(page);
    await page.goto(`/administrare/locatie/${id}`);
    await page.getByRole('button', { name: 'Arhivează' }).click();
    const dlg = page.getByRole('alertdialog');
    await expect(dlg).toContainText('istoricul ei rămân în statistici');
    await dlg.getByRole('button', { name: 'Arhivează' }).click();
    await expect(page).toHaveURL(/\/administrare$/);
    await expect(page.locator('.loc-list')).not.toContainText('Arhivă E2E');
    await page.goto('/');
    await expect(page.locator('.loc-card', { hasText: 'Cahul' })).toBeVisible();
    await expect(page.locator('.loc-card', { hasText: 'Arhivă E2E' })).toHaveCount(0);
  });

  test('ștergere definitivă', async ({ page }) => {
    const api = await adminApi();
    const id = await createLocation(api, { name: 'Ștergere E2E', devices: [{ name: 'R', host: '10.0.6.6' }] });
    await api.dispose();
    await login(page);
    await page.goto(`/administrare/locatie/${id}`);
    await page.getByRole('button', { name: 'Șterge definitiv' }).click();
    const dlg = page.getByRole('alertdialog');
    await expect(dlg).toContainText('ireversibilă');
    const confirm = dlg.getByRole('button', { name: 'Șterge definitiv' });
    await expect(confirm).toBeDisabled();
    await dlg.getByRole('textbox').fill('Ștergere E2');
    await expect(confirm).toBeDisabled();
    await dlg.getByRole('textbox').fill('Ștergere E2E');
    await confirm.click();
    await expect(page).toHaveURL(/\/administrare$/);
    await expect(page.locator('.loc-list')).not.toContainText('Ștergere E2E');
    await page.goto(`/administrare/locatie/${id}`);
    await expect(page.getByText('Locația nu există sau a fost arhivată.')).toBeVisible();
  });

  test('locații arhivate: Statistici și ștergere definitivă', async ({ page }) => {
    const api = await adminApi();
    const id = await createLocation(api, { name: 'Arhivată E2E', devices: [{ name: 'R', host: '10.0.6.7' }] });
    await api.send('DELETE', `/api/admin/locations/${id}`);
    await api.dispose();

    // Statistici: hidden by default, shown with "Include arhivate"
    await page.goto('/statistici');
    const loc = page.getByLabel('Locație');
    await expect(loc.locator('option', { hasText: 'Cahul' })).toHaveCount(1);
    await expect(loc.locator('option', { hasText: 'Arhivată E2E' })).toHaveCount(0);
    await page.getByLabel('Include arhivate').check();
    await expect(page).toHaveURL(/archived=1/);
    await expect(loc.locator('option', { hasText: 'Arhivată E2E (arhivat)' })).toHaveCount(1);
    // a link to an archived location turns the switch on
    await page.goto(`/statistici?location=${id}`);
    await expect(page.getByLabel('Include arhivate')).toBeChecked();
    await expect(loc).toHaveValue(String(id));
    // switching it off drops the hidden selection
    await page.getByLabel('Include arhivate').uncheck();
    await expect(loc).toHaveValue('');
    await expect(page).not.toHaveURL(/location=/);

    // Administrare: archived section with permanent delete
    await login(page);
    const section = page.locator('details.archived');
    await section.locator('summary').click();
    const row = section.locator('li', { hasText: 'Arhivată E2E' });
    await expect(row).toContainText('arhivată la');
    await row.getByRole('button', { name: 'Șterge definitiv' }).click();
    const dlg = page.getByRole('alertdialog');
    await dlg.getByRole('textbox').fill('Arhivată E2E');
    await dlg.getByRole('button', { name: 'Șterge definitiv' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'a fost ștearsă definitiv' })).toBeVisible();
    await expect(page.locator('.arch-list li', { hasText: 'Arhivată E2E' })).toHaveCount(0);
  });

  test('coordonate din hartă', async ({ page }) => {
    await login(page);
    await page.getByRole('link', { name: 'Adaugă locație' }).click();
    await page.locator('.coord-map .leaflet-container').click({ position: { x: 150, y: 120 } });
    await expect(page.getByLabel('Latitudine')).not.toHaveValue('');
    await expect(page.getByLabel('Longitudine')).not.toHaveValue('');
  });

  test('ordinea manuală a locațiilor', async ({ page }) => {
    await login(page);
    const list = page.locator('.loc-list li');
    const first = await list.nth(0).locator('a').innerText();
    await list.nth(0).getByRole('button', { name: 'Mută în jos' }).click();
    await expect(list.nth(1).locator('a')).toHaveText(first);
    await page.reload();
    await expect(page.locator('.loc-list li').nth(1).locator('a')).toHaveText(first);
    await page.locator('.loc-list li').nth(1).getByRole('button', { name: 'Mută în sus' }).click();
    await expect(page.locator('.loc-list li').nth(0).locator('a')).toHaveText(first);
  });
});

test('sesiune expirată în editor păstrează modificările', async ({ page, context }) => {
  await login(page);
  const id = await locationId('Cahul');
  await page.goto(`/administrare/locatie/${id}`);
  await expect(page.locator('.device-row').first()).toBeVisible();
  await page.getByLabel('Denumire', { exact: true }).fill('Cahul expirat');
  await context.clearCookies();
  await page.getByRole('button', { name: 'Salvează' }).click();
  const dlg = page.getByRole('dialog', { name: 'Autentificare administrator' });
  await expect(dlg).toContainText('Sesiunea a expirat');
  await dlg.getByLabel('Parola', { exact: true }).fill(PASSWORD);
  await dlg.getByRole('button', { name: 'Autentificare' }).click();
  await expect(dlg).toHaveCount(0);
  await expect(page.getByLabel('Denumire', { exact: true })).toHaveValue('Cahul expirat');
  await page.getByLabel('Denumire', { exact: true }).fill('Cahul');
  await page.getByRole('button', { name: 'Salvează' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Locația a fost salvată.' })).toBeVisible();
});

test('dezactivare temporară a unui echipament', async ({ page }) => {
  const api = await adminApi();
  const id = await createLocation(api, { name: 'Dezactivare E2E', devices: [{ name: 'Router', host: '10.0.9.1' }, { name: 'Camere', host: '10.0.9.2' }] });
  try {
    await login(page);
    await page.goto(`/administrare/locatie/${id}`);
    const row = page.locator('.device-row').nth(1);
    await row.getByLabel('Dezactivat').check();
    await row.getByRole('button', { name: '+1 zi' }).click();
    await expect(row.getByRole('status')).toContainText('Se reactivează automat la');
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Locația a fost salvată.' })).toBeVisible();

    await page.goto('/administrare');
    await expect(page.locator('.loc-list li', { hasText: 'Dezactivare E2E' })).toContainText('1 dezactivat');

    await page.goto('/');
    const card = page.locator('.loc-card', { hasText: 'Dezactivare E2E' });
    await expect(card).toContainText('Router');
    await expect(card).not.toContainText('Camere');

    // Reactivare manuală: echipamentul revine pe card.
    await page.goto(`/administrare/locatie/${id}`);
    await expect(page.locator('.device-row').nth(1).getByLabel('Dezactivat')).toBeChecked();
    await page.locator('.device-row').nth(1).getByLabel('Dezactivat').uncheck();
    await page.getByRole('button', { name: 'Salvează' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Locația a fost salvată.' })).toBeVisible();
    await page.goto('/');
    await page.getByRole('button', { name: 'Compact' }).click();
    await expect(page.locator('.loc-card', { hasText: 'Dezactivare E2E' })).toContainText('Camere');
  } finally {
    await api.send('DELETE', `/api/admin/locations/${id}`);
    await api.dispose();
  }
});
