import { expect, locationId, setHost, test, waitState } from './fixtures';

test.describe('Statistici', () => {
  test('tabele după o cădere, fără carduri de rezumat', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓ Funcțională');
    await setHost('10.0.1.1', 'timeout');
    await waitState(page, 'Cahul', '✕ Indisponibilă');
    await setHost('10.0.1.1', 'ok');
    await waitState(page, 'Cahul', '✓ Funcțională');
    await page.goto('/statistici?preset=24h');
    const row = page.locator('table.devices tbody tr', { hasText: 'Cahul' });
    await expect(row).toBeVisible();
    await expect(row.locator('td.incidents')).not.toHaveText('0');
    await expect(page.locator('.kpis')).toHaveCount(0);
    const loc = page.locator('table.locs tbody tr', { hasText: 'Cahul' });
    await expect(loc.locator('td').nth(1)).toHaveText(/^\d{1,3},\d{2}%$/);
    // when it was unavailable: duration, count, timeline and the period itself
    await expect(loc.locator('td').nth(2)).not.toHaveText('—');
    await expect(loc.locator('td').nth(3)).not.toHaveText('0');
    await expect(loc.locator('.seg.f-down')).not.toHaveCount(0);
    const outage = page.locator('table.outages tbody tr', { hasText: 'Cahul' }).first();
    await expect(outage).toContainText('Indisponibilă');
    await expect(outage.locator('td').nth(5)).not.toHaveText('—');
    await page.getByLabel('Include perioadele degradate').check();
    await expect(page).toHaveURL(/degraded=1/);
  });

  test('filtrele sunt păstrate în URL', async ({ page }) => {
    const id = await locationId('Cahul');
    await page.goto('/statistici');
    await page.getByLabel('Locație').selectOption({ label: 'Cahul' });
    await expect(page).toHaveURL(new RegExp(`location=${id}`));
    await page.reload();
    await expect(page.getByLabel('Locație')).toHaveValue(String(id));
    const rows = page.locator('table.devices tbody tr');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText('Cahul');
  });

  test('sortare după disponibilitate', async ({ page }) => {
    await page.goto('/statistici');
    const th = page.locator('table.devices th', { hasText: 'Disponibilitate' });
    await th.getByRole('button').click();
    await expect(th).toHaveAttribute('aria-sort', 'ascending');
    await th.getByRole('button').click();
    await expect(th).toHaveAttribute('aria-sort', 'descending');
  });

  test('perioadă personalizată invalidă', async ({ page }) => {
    await page.goto('/statistici');
    await expect(page.locator('table.devices tbody tr').first()).toBeVisible();
    await page.getByRole('button', { name: 'Personalizată' }).click();
    let sent = false;
    page.on('request', (r) => {
      if (r.url().includes('/api/stats')) sent = true;
    });
    await page.getByLabel('Început — data').fill('2026-11-05');
    await page.getByLabel('Început — ora').fill('10:00');
    await page.getByLabel('Sfârșit — data').fill('2026-11-04');
    await page.getByLabel('Sfârșit — ora').fill('10:00');
    await page.getByRole('button', { name: 'Aplică' }).click();
    await expect(page.getByText('Sfârșitul trebuie să fie după început.')).toBeVisible();
    expect(sent).toBe(false);
  });

  test('perioadă personalizată validă ajunge în URL', async ({ page }) => {
    await page.goto('/statistici');
    await page.getByRole('button', { name: 'Personalizată' }).click();
    await page.getByLabel('Început — data').fill('2026-11-01');
    await page.getByLabel('Început — ora').fill('00:00');
    await page.getByLabel('Sfârșit — data').fill('2026-11-02');
    await page.getByLabel('Sfârșit — ora').fill('00:00');
    await page.getByRole('button', { name: 'Aplică' }).click();
    // 2026-11-01 00:00 in Chisinau = 2026-10-31 22:00 UTC
    await expect(page).toHaveURL(new RegExp(`from=${Date.UTC(2026, 9, 31, 22)}&to=${Date.UTC(2026, 10, 1, 22)}`));
  });

  test('fusul orar este afișat', async ({ page }) => {
    await page.goto('/statistici');
    await expect(page.getByText('Ora: Europe/Chisinau')).toBeVisible();
  });
});

test('raportul nu se reîncarcă la fiecare ping', async ({ page }) => {
  test.setTimeout(40_000);
  await page.goto('/statistici');
  await expect(page.locator('table.devices tbody tr').first()).toBeVisible();
  let n = 0;
  page.on('request', (r) => {
    if (r.url().includes('/api/stats')) n++;
  });
  // the server pings every 15 s; a ping updates the clock offset
  await page.waitForTimeout(16_500);
  expect(n).toBe(0);
});
