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

test('mesajul de stare stă pe rândul numelui, fără să-l acopere', async ({ page }) => {
  const api = await adminApi();
  const id = await createLocation(api, { name: 'Mesaj E2E', devices: [{ name: 'Router principal de bază', host: '10.0.9.9' }] });
  await setHost('10.0.9.9', 'timeout');
  try {
    // Row geometry: the name on one line and whole, the message on the same
    // line after it, the IP either clear of the message or hidden entirely.
    const check = () =>
      page.locator(`.loc-card[data-id="${id}"] li.has-msg`).evaluate((li) => {
        const r = document.createRange();
        r.selectNodeContents(li.querySelector('.dname')!);
        const name = r.getBoundingClientRect();
        const lines = r.getClientRects();
        const msg = li.querySelector('.msg')!.getBoundingClientRect();
        const host = li.querySelector('.host')!.getBoundingClientRect();
        const hostHidden = getComputedStyle(li.querySelector('.host')!).opacity === '0';
        return {
          mode: ['between', 'cover', 'below'].find((m) => li.classList.contains(m)),
          nameOneLine: [...lines].every((l) => Math.abs(l.top - lines[0].top) < 2),
          sameLine: Math.abs((msg.top + msg.bottom) / 2 - (name.top + name.bottom) / 2) < 3,
          clearOfName: msg.left >= name.right - 0.5,
          ipOk: hostHidden || msg.right <= host.left + 0.5,
        };
      });
    for (const width of [1400, 380]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto('/');
      await expect(page.locator(`.loc-card[data-id="${id}"] .msg`)).toBeVisible({ timeout: 20_000 });
      await expect.poll(check).toMatchObject({ mode: 'cover', nameOneLine: true, sameLine: true, clearOfName: true, ipOk: true });
    }
  } finally {
    await api.send('DELETE', `/api/admin/locations/${id}`);
    await api.dispose();
  }
});
