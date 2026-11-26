const STORAGE_KEY = 'sasmita.usage.always.v1';

function read(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

/** "Always show usage" is UI state only; no migration and no IPC command. */
class UsageSettings {
  alwaysShow = $state(read());

  set(value: boolean): void {
    this.alwaysShow = value;
    try {
      localStorage.setItem(STORAGE_KEY, value ? '1' : '0');
    } catch {
      // Storage can be unavailable in a private WebView; the toggle still works.
    }
  }

  toggle(): void {
    this.set(!this.alwaysShow);
  }
}

export const usageSettings = new UsageSettings();
