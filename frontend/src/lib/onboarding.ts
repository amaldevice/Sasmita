import { bind } from 'mygo-runtime';
import type { Locale } from './i18n.svelte';

export interface LocalEndpoint {
  kind: string;
  name: string;
  baseUrl: string;
  apiBaseUrl: string;
}

/** Declarative data from internal/onboarding; VisualHint is for later tickets. */
export interface ExamplePrompt {
  id: string;
  prompt: string;
  titleId: string;
  titleEn: string;
  detailId: string;
  detailEn: string;
  visualHint: string;
}

export interface OnboardingIPC {
  listExamplePrompts(): Promise<ExamplePrompt[]>;
  detectLocalEndpoints(): Promise<LocalEndpoint[]>;
}

/** Typed binding to shellService.Get* (mirrors desktop/onboarding.go). */
const bindings = {
  listExamplePrompts: bind<[], ExamplePrompt[]>('shellService.ListExamplePrompts', () => []),
  detectLocalEndpoints: bind<[], LocalEndpoint[]>('shellService.DetectLocalEndpoints', () => []),
};

export class MyGoOnboardingIPC implements OnboardingIPC {
  async listExamplePrompts(): Promise<ExamplePrompt[]> {
    try {
      const prompts = await bindings.listExamplePrompts();
      if (prompts?.length) return prompts;
    } catch {
      // A first-run shell may answer before the binding is ready. The catalog
      // is declarative data, so the UI can still show the examples.
    }
    return structuredClone(fallbackPrompts);
  }

  detectLocalEndpoints(): Promise<LocalEndpoint[]> {
    return bindings.detectLocalEndpoints();
  }
}

// Fake fixtures mirror the Go catalog. Only the fake's probe is controllable;
// no browser test opens a socket to a local port.
const fallbackPrompts: ExamplePrompt[] = [
  {
    id: 'training-flow',
    prompt: 'Jelaskan alur training neural network dengan diagram',
    titleId: 'Alur training neural network',
    titleEn: 'Neural-network training flow',
    detailId: 'Lihat langkahnya sebagai alur',
    detailEn: 'See the steps as a flow',
    visualHint: 'flow',
  },
  {
    id: 'sales-trend',
    prompt: 'Tunjukkan tren penjualan per bulan dari workbook ini',
    titleId: 'Tren penjualan bulanan',
    titleEn: 'Monthly sales trend',
    detailId: 'Ubah angka menjadi grafik',
    detailEn: 'Turn the numbers into a chart',
    visualHint: 'chart',
  },
  {
    id: 'research-compare',
    prompt: 'Bandingkan dua pendekatan riset ini dalam tabel',
    titleId: 'Bandingkan dua pendekatan',
    titleEn: 'Compare two approaches',
    detailId: 'Perbedaan yang mudah dibaca',
    detailEn: 'Differences at a glance',
    visualHint: 'comparison',
  },
  {
    id: 'decision-timeline',
    prompt: 'Ubah catatan rapat saya menjadi linimasa keputusan',
    titleId: 'Linimasa keputusan rapat',
    titleEn: 'Meeting decision timeline',
    detailId: 'Urutan keputusan dari catatan',
    detailEn: 'Decisions in order from your notes',
    visualHint: 'timeline',
  },
];

const fakeEndpoints: LocalEndpoint[] = [
  {
    kind: 'ollama', name: 'Ollama',
    baseUrl: 'http://127.0.0.1:11434', apiBaseUrl: 'http://127.0.0.1:11434/v1',
  },
  {
    kind: 'llama.cpp', name: 'llama.cpp',
    baseUrl: 'http://127.0.0.1:8080', apiBaseUrl: 'http://127.0.0.1:8080/v1',
  },
];

export class FakeOnboardingIPC implements OnboardingIPC {
  async listExamplePrompts(): Promise<ExamplePrompt[]> {
    return structuredClone(fallbackPrompts);
  }

  async detectLocalEndpoints(): Promise<LocalEndpoint[]> {
    const detect = new URLSearchParams(location.search).get('detect');
    if (!detect || detect === 'none') return [];
    if (detect === 'all') return structuredClone(fakeEndpoints);
    return structuredClone(fakeEndpoints.filter((endpoint) => endpoint.kind === detect));
  }
}

export function createOnboardingIPC(): OnboardingIPC {
  const params = new URLSearchParams(location.search);
  if (params.get('fake') === '1') return new FakeOnboardingIPC();
  return new MyGoOnboardingIPC();
}

// First-run state is UI state only; no migration is needed for v1.
const DONE_KEY = 'sasmita.onboarding.v1';

export function completeOnboarding(): void {
  try {
    localStorage.setItem(DONE_KEY, '1');
  } catch {
    // Storage can be unavailable in a private WebView; the screen still closes.
  }
}

export function shouldShowOnboarding(): boolean {
  const params = new URLSearchParams(location.search);
  // Existing fake-IPC tests start in a chat; only an explicit flag shows setup.
  if (params.get('fake') === '1') return params.get('onboarding') === '1';
  try {
    return localStorage.getItem(DONE_KEY) !== '1';
  } catch {
    return true;
  }
}

/**
 * Onboarding copy lives with the flow so a shell/i18n lane can change the
 * global string table without touching this screen. Both locales are always
 * available.
 */
export const onboardingCopy = {
  id: {
    dialogLabel: 'Pengaturan awal Sasmita',
    welcomeTitle: 'Sasmita membantu Anda memahami, bukan sekadar membaca.',
    welcomeBody: 'Tanyakan apa saja. Jawaban muncul sebagai visual yang bisa dipahami, lalu teks. Percakapan Anda tersimpan di komputer sendiri.',
    detected: 'Model lokal terdeteksi',
    notTested: 'Terdeteksi, belum diuji',
    noLocal: 'Belum ada model lokal yang berjalan. Anda tetap bisa memakai API.',
    useApi: 'Pakai API',
    apiTitle: 'Hubungkan API',
    apiBody: 'Pilih jenis penyedia, isi base URL dan kunci API. Kunci disimpan di Keychain, lalu koneksi diuji sebelum Anda melanjutkan.',
    kindLabel: 'Jenis penyedia',
    nameLabel: 'Nama',
    baseUrlLabel: 'Base URL',
    keyLabel: 'Kunci API',
    localKeyHint: 'Model lokal biasanya tidak memerlukan kunci API.',
    saveAndTest: 'Simpan dan uji koneksi',
    continueLabel: 'Lanjut ke chat',
    back: 'Kembali',
    working: 'Menguji…',
    fieldsRequired: 'Isi nama dan base URL.',
    keyRequired: 'Isi kunci API untuk penyedia remote.',
    emptyTitle: 'Mulai dari pertanyaan pertama',
    emptyBody: 'Pilih salah satu contoh di bawah atau tulis pertanyaan Anda sendiri.',
  },
  en: {
    dialogLabel: 'Sasmita setup',
    welcomeTitle: 'Sasmita helps you understand, not just read.',
    welcomeBody: 'Ask anything. Answers arrive as a visual you can grasp, then plain text. Your conversations stay on your computer.',
    detected: 'Detected local models',
    notTested: 'Detected, not tested',
    noLocal: 'No local model is running. You can still connect an API.',
    useApi: 'Use an API',
    apiTitle: 'Connect an API',
    apiBody: 'Choose a provider type, enter the base URL and API key. The key goes to the Keychain, then the connection is tested before you continue.',
    kindLabel: 'Provider type',
    nameLabel: 'Name',
    baseUrlLabel: 'Base URL',
    keyLabel: 'API key',
    localKeyHint: 'Local models usually do not need an API key.',
    saveAndTest: 'Save and test connection',
    continueLabel: 'Continue to chat',
    back: 'Back',
    working: 'Testing…',
    fieldsRequired: 'Enter a name and base URL.',
    keyRequired: 'Enter an API key for a remote provider.',
    emptyTitle: 'Start with a first question',
    emptyBody: 'Pick one of the examples below or write your own question.',
  },
} as const satisfies Record<Locale, Record<string, string>>;
