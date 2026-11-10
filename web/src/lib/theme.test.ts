import { describe, expect, it } from 'vitest';
import { effectiveTheme, initialTheme } from './theme';

describe('initialTheme', () => {
  it('dispatch defaults to dark', () => expect(initialTheme(null, false, 'dispatch')).toBe('dark'));
  it('stored preference wins', () => expect(initialTheme('light', true, 'dispatch')).toBe('light'));
  it('follows the system otherwise', () => {
    expect(initialTheme(null, true, 'monitor')).toBe('dark');
    expect(initialTheme(null, false, 'monitor')).toBe('light');
  });
  it('ignores invalid stored values', () => expect(initialTheme('violet', false, 'monitor')).toBe('light'));
});

describe('effectiveTheme', () => {
  it('dispatch is dark unless the user chose a theme', () => {
    expect(effectiveTheme(null, false, 'dispatch')).toBe('dark');
    expect(effectiveTheme('light', true, 'dispatch')).toBe('light');
  });
  it('other pages follow the system', () => {
    expect(effectiveTheme(null, true, 'monitor')).toBe('dark');
    expect(effectiveTheme(null, false, 'stats')).toBe('light');
  });
});
