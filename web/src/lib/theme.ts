export type Theme = 'light' | 'dark';

const KEY = 'nw-theme';

export function initialTheme(stored: string | null, prefersDark: boolean, routeName: string): Theme {
  if (stored === 'light' || stored === 'dark') return stored;
  if (routeName === 'dispatch') return 'dark';
  return prefersDark ? 'dark' : 'light';
}

export function storedTheme(): string | null {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function saveTheme(t: Theme): void {
  try {
    localStorage.setItem(KEY, t);
  } catch {
    /* storage unavailable: the choice lasts for this page only */
  }
}

export function applyTheme(t: Theme): void {
  document.documentElement.dataset.theme = t;
}

/** Theme when the user has not chosen one explicitly (stored wins if valid). */
export function effectiveTheme(stored: string | null, prefersDark: boolean, routeName: string): Theme {
  return initialTheme(stored, prefersDark, routeName);
}

export function validTheme(v: string | null): Theme | null {
  return v === 'light' || v === 'dark' ? v : null;
}
