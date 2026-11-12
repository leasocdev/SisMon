import { readFileSync } from 'node:fs';
import { adminApi, expect, locationId, login, test } from './fixtures';

// Minimal Windows-1250 encoder for the characters used below.
const CP1250: Record<string, number> = { ă: 0xe3, â: 0xe2, î: 0xee, ş: 0xba, ţ: 0xfe, Ş: 0xaa, Ţ: 0xde, Ă: 0xc3 };
function cp1250(s: string): Buffer {
  return Buffer.from([...s].map((c) => CP1250[c] ?? c.charCodeAt(0)));
}

const file = (name: string, buffer: Buffer) => ({ name, mimeType: 'text/csv', buffer });

test.describe('Import și export', () => {
  test('previzualizare cu erori pe celulă', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/import');
    const csv = cp1250('Locaţie;Echipament;IP;Primar\r\nImport Bun;Router;10.0.30.1;da\r\nImport Rău;Router;999.1.1.1;da\r\n');
    await page.getByLabel('Fișier CSV sau Excel').setInputFiles(file('excel.csv', csv));
    await page.getByRole('button', { name: 'Previzualizează' }).click();
    await expect(page.locator('.summary')).toContainText('windows-1250');
    const bad = page.locator('tr', { hasText: 'Import Rău' });
    await expect(bad.locator('td[aria-invalid="true"]')).toContainText('999.1.1.1');
    await expect(bad).toContainText('Adresă IP sau nume DNS invalid');
    await expect(page.getByRole('button', { name: 'Importă' })).toBeDisabled();
  });

  test('import reușit', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/import');
    const csv = Buffer.from('location,device,host\nImport E2E Nord,Router,10.0.31.1\nImport E2E Sud,Router,10.0.32.1\n');
    await page.getByLabel('Fișier CSV sau Excel').setInputFiles(file('l.csv', csv));
    await page.getByRole('button', { name: 'Previzualizează' }).click();
    await page.getByRole('button', { name: 'Importă' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'S-au importat 2 locații.' })).toBeVisible();
    await page.goto('/');
    await expect(page.locator('.loc-card', { hasText: 'Import E2E Nord' })).toBeVisible();
    const api = await adminApi();
    for (const n of ['Import E2E Nord', 'Import E2E Sud']) await api.send('DELETE', `/api/admin/locations/${await locationId(n)}`);
    await api.dispose();
  });

  test('exportul locațiilor se poate reimporta', async ({ page }) => {
    await login(page);
    await page.goto('/administrare/import');
    const dl = page.waitForEvent('download');
    await page.getByRole('link', { name: 'Exportă locațiile (CSV)' }).click();
    const path = await (await dl).path();
    const content = readFileSync(path);
    expect(content.toString('utf8')).toContain('Locație;Adresă;Latitudine;Longitudine;Echipament;Host;Tip;Primar;Observații');
    expect(content.toString('utf8')).toContain('Cahul');
    await page.getByLabel('Fișier CSV sau Excel').setInputFiles(file('locatii.csv', content));
    await page.getByRole('button', { name: 'Previzualizează' }).click();
    await expect(page.locator('tr', { hasText: 'Cahul' }).first()).toContainText('Locația există deja');
  });

  test('exporturile statisticilor', async ({ page }) => {
    await page.goto('/statistici?preset=7z');
    const csvLink = page.getByRole('link', { name: 'CSV' });
    await expect(csvLink).toHaveAttribute('href', /\/api\/export\/csv\?.*from=\d+.*to=\d+/);
    const d1 = page.waitForEvent('download');
    await csvLink.click();
    const csv = await d1;
    expect(csv.suggestedFilename()).toMatch(/^sismon-statistici-\d{8}-\d{4}\.csv$/);
    expect(readFileSync(await csv.path()).subarray(0, 3).toString('hex')).toBe('efbbbf');
    const d2 = page.waitForEvent('download');
    await page.getByRole('link', { name: 'PDF' }).click();
    expect(readFileSync(await (await d2).path()).subarray(0, 5).toString()).toBe('%PDF-');
  });
});
