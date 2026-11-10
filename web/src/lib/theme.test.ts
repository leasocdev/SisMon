import { describe, expect, it } from 'vitest';
import { initialTheme } from './theme';

describe('initialTheme', () => {
  it('dispatch defaults to dark', () => expect(initialTheme(null, false, 'dispatch')).toBe('dark'));
  it('stored preference wins', () => expect(initialTheme('light', true, 'dispatch')).toBe('light'));
  it('follows the system otherwise', () => {
    expect(initialTheme(null, true, 'monitor')).toBe('dark');
    expect(initialTheme(null, false, 'monitor')).toBe('light');
  });
  it('ignores invalid stored values', () => expect(initialTheme('violet', false, 'monitor')).toBe('light'));
});
