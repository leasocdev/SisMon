import { expect, test } from './fixtures';

test('aplicația se încarcă pe paginile principale', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/NetWatch/);
  await expect(page.getByRole('navigation', { name: 'Navigare principală' })).toContainText('Monitorizare');
  await expect(page.getByRole('status').first()).toContainText('Conectat');

  for (const path of ['/dispecerat', '/administrare', '/statistici']) {
    const res = await page.goto(path);
    expect(res?.status()).toBe(200);
    await expect(page.locator('#app > *').first()).toBeVisible();
  }
});

test.describe('fixture', () => {
  test.fail();
  test('detectează o încălcare CSP (atribut style inline)', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => {
      const d = document.createElement('div');
      d.innerHTML = '<span style="color:red">x</span>';
      document.body.appendChild(d);
    });
    await page.waitForTimeout(200);
  });
});

test.describe('încărcare eșuată', () => {
  test.use({ allowConsole: [/Failed to fetch dynamically imported module|Importing a module script failed|net::ERR_FAILED/] });
  test('o pagină care nu se poate încărca afișează un mesaj', async ({ page }) => {
    await page.goto('/');
    await page.route(/\/assets\/(MapPage|Stats)-.*\.js$/, (r) => r.abort());
    await page.getByRole('link', { name: 'Statistici' }).click();
    await expect(page.getByText('Pagina nu a putut fi încărcată')).toBeVisible();
  });
});

test('bannerul modului demonstrativ', async ({ page }) => {
  await page.goto('http://127.0.0.1:18511/');
  await expect(page.getByText('MOD DEMONSTRATIV')).toBeVisible();
  await page.goto('http://127.0.0.1:18511/dispecerat');
  await expect(page.getByText('MOD DEMONSTRATIV')).toBeVisible();
  await page.goto('/');
  await expect(page.locator('.loc-card').first()).toBeVisible();
  await expect(page.getByText('MOD DEMONSTRATIV')).toHaveCount(0);
});
