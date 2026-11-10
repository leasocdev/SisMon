import { adminApi, createLocation, expect, setHost, test, waitState } from './fixtures';

test.describe('Monitorizare', () => {
  test('cardurile apar sortate după gravitate', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓ Funcțională');
    await setHost('10.0.1.1', 'timeout');
    await waitState(page, 'Cahul', '✕ Indisponibilă');
    await expect(page.locator('.loc-card').first()).toContainText('Cahul');
  });

  test('confirmarea căderii este afișată', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓ Funcțională');
    await setHost('10.0.1.1', 'timeout');
    await expect(page.locator('.loc-card', { hasText: 'Cahul' })).toContainText('cădere în confirmare', { timeout: 10_000 });
  });

  test('secundar căzut → locație degradată', async ({ page }) => {
    await page.goto('/');
    await setHost('10.0.2.2', 'timeout');
    await waitState(page, 'Bălți', '▲ Degradată');
  });

  test('eroare monitor pe un echipament', async ({ page }) => {
    await page.goto('/');
    await setHost('10.0.0.1', 'permission');
    await expect(page.locator('.loc-card', { hasText: 'Chișinău Centru' })).toContainText('! Eroare monitor', { timeout: 20_000 });
  });

  test('filtrul pe stări', async ({ page }) => {
    await page.goto('/');
    await setHost('10.0.1.1', 'timeout');
    await waitState(page, 'Cahul', '✕ Indisponibilă');
    const down = page.getByRole('button', { name: /Indisponibile/ });
    await down.click();
    await expect(down).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('.loc-card')).toHaveCount(1);
    await expect(page.locator('.loc-card')).toContainText('Cahul');
    await down.click();
    await expect(page.locator('.loc-card')).not.toHaveCount(1);
  });

  test('căutare după IP', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('searchbox', { name: /Căutare/ }).fill('10.0.2.2');
    await expect(page.locator('.loc-card')).toHaveCount(1);
    await expect(page.locator('.loc-card')).toContainText('Bălți');
  });

  test('căutare fără diacritice', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('searchbox', { name: /Căutare/ }).fill('chisinau');
    await expect(page.locator('.loc-card')).toHaveCount(1);
  });

  test('nume cu HTML afișat literal', async ({ page }) => {
    const api = await adminApi();
    const name = '<img src=x onerror=window.__xss=1>Ștefan Vodă';
    const id = await createLocation(api, { name, devices: [{ name: 'R<b>1</b>', host: '10.0.7.1' }] });
    try {
      await page.goto('/');
      const card = page.locator('.loc-card', { hasText: 'Ștefan Vodă' });
      await expect(card).toContainText(name);
      await expect(card).toContainText('R<b>1</b>');
      expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined();
    } finally {
      await api.send('DELETE', `/api/admin/locations/${id}`);
      await api.dispose();
    }
  });

  test('anunț aria-live la schimbarea stării', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓ Funcțională');
    await setHost('10.0.1.1', 'timeout');
    await expect(page.locator('[aria-live=polite]').filter({ hasText: 'Cahul: ✕ Indisponibilă' })).toHaveCount(1, { timeout: 20_000 });
  });

  test('navigare din tastatură spre panou', async ({ page }) => {
    await page.goto('/');
    await waitState(page, 'Cahul', '✓');
    const card = page.locator('.loc-card', { hasText: 'Cahul' });
    for (let i = 0; i < 40; i++) {
      await page.keyboard.press('Tab');
      if (await card.evaluate((el) => el === document.activeElement)) break;
    }
    await expect(card).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/locatie\/\d+$/);
  });

  test('mini-cronologia este afișată', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.loc-card', { hasText: 'Cahul' }).locator('.mini-timeline .seg').first()).toBeAttached();
  });
});
