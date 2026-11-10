import { deflateSync } from 'node:zlib';
import { expect, login, test } from './fixtures';

function crc32(buf: Buffer): number {
  let c = ~0;
  for (const b of buf) {
    c ^= b;
    for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1));
  }
  return ~c >>> 0;
}

function chunk(type: string, data: Buffer): Buffer {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const td = Buffer.concat([Buffer.from(type), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(td));
  return Buffer.concat([len, td, crc]);
}

/** A valid 64×64 RGB PNG. */
function png(): Buffer {
  const w = 64;
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(w, 4);
  ihdr[8] = 8;
  ihdr[9] = 2;
  const raw = Buffer.alloc((w * 3 + 1) * w, 0x80);
  for (let y = 0; y < w; y++) raw[y * (w * 3 + 1)] = 0;
  return Buffer.concat([Buffer.from('89504e470d0a1a0a', 'hex'), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0))]);
}

test('logo: încărcare, eroare, eliminare', async ({ page }) => {
  await login(page);
  await page.goto('/administrare/identitate');
  await page.getByLabel('Fișier PNG').setInputFiles({ name: 'text.png', mimeType: 'image/png', buffer: Buffer.from('nu e imagine') });
  await page.getByRole('button', { name: 'Încarcă logo-ul' }).click();
  await expect(page.getByText('Logo-ul trebuie să fie o imagine PNG.')).toBeVisible();
  await page.getByLabel('Fișier PNG').setInputFiles({ name: 'logo.png', mimeType: 'image/png', buffer: png() });
  await page.getByRole('button', { name: 'Încarcă logo-ul' }).click();
  await expect(page.locator('.hdr img.logo')).toHaveAttribute('src', /\/branding\/logo\?v=\d+/);
  await page.getByRole('button', { name: 'Elimină logo-ul' }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: 'Elimină' }).click();
  await expect(page.locator('.hdr img.logo')).toHaveCount(0);
});
