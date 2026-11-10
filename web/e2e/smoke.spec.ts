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
