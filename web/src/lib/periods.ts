import { utcToZonedParts, zonedToUtc } from './time';

export type PresetKey = '24h' | '7z' | '30z' | 'luna' | 'luna-trecuta';

export const PRESETS: PresetKey[] = ['24h', '7z', '30z', 'luna', 'luna-trecuta'];

export const PRESET_LABEL: Record<PresetKey, string> = {
  '24h': '24 h',
  '7z': '7 zile',
  '30z': '30 zile',
  luna: 'Luna curentă',
  'luna-trecuta': 'Luna trecută',
};

const DAY = 86_400_000;

function monthStart(y: number, m: number): number {
  return zonedToUtc(`${y}-${String(m).padStart(2, '0')}-01`, '00:00');
}

export function presetRange(key: PresetKey, now: number): { from: number; to: number } {
  switch (key) {
    case '24h':
      return { from: now - DAY, to: now };
    case '7z':
      return { from: now - 7 * DAY, to: now };
    case '30z':
      return { from: now - 30 * DAY, to: now };
  }
  const [y, m] = utcToZonedParts(now).date.split('-').map(Number);
  if (key === 'luna') return { from: monthStart(y, m), to: now };
  const py = m === 1 ? y - 1 : y;
  const pm = m === 1 ? 12 : m - 1;
  return { from: monthStart(py, pm), to: monthStart(y, m) };
}
