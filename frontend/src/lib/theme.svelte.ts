export type Theme = 'dark' | 'light';

const STORAGE_KEY = 'sasmita.theme';

function initialTheme(): Theme {
  if (typeof localStorage === 'undefined') return 'dark';
  return localStorage.getItem(STORAGE_KEY) === 'light' ? 'light' : 'dark';
}

/** DESIGN.md tokens are keyed off data-theme on the root element. */
class ThemeStore {
  theme = $state<Theme>(initialTheme());

  constructor() {
    if (typeof document !== 'undefined') this.apply();
  }

  private apply(): void {
    document.documentElement.dataset.theme = this.theme;
  }

  set(theme: Theme): void {
    this.theme = theme;
    if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, theme);
    if (typeof document !== 'undefined') this.apply();
  }

  toggle(): void {
    this.set(this.theme === 'dark' ? 'light' : 'dark');
  }
}

export const theme = new ThemeStore();
