export type Locale = 'id' | 'en';

const STORAGE_KEY = 'sasmita.locale';

const strings = {
  id: {
    newChat: 'Percakapan baru',
    search: 'Cari',
    projects: 'Proyek',
    artifacts: 'Artifact',
    settings: 'Pengaturan',
    recent: 'TERBARU',
    collapse: 'Ciutkan sidebar',
    expand: 'Luaskan sidebar',
    modeChat: 'Mode Chat',
    rename: 'Ubah nama',
    move: 'Pindahkan',
    openFolder: 'Buka folder',
    save: 'Simpan',
    cancel: 'Batal',
    noProject: 'Tanpa proyek',
    project: 'Proyek',
    startConversation: 'Mulai percakapan',
    askFirst: 'Ajukan pertanyaan pertama Anda.',
    you: 'Anda',
    working: 'Bekerja…',
    cache: 'cache',
    message: 'Pesan',
    send: 'Kirim',
    askAnything: 'Tanya sesuatu…',
    searchConversations: 'Cari percakapan',
    noResults: 'Tidak ada hasil',
    noProjects: 'Belum ada proyek',
    noArtifacts: 'Belum ada artifact',
    theme: 'Tema',
    dark: 'Gelap',
    light: 'Terang',
    language: 'Bahasa',
    indonesian: 'Bahasa Indonesia',
    english: 'English',
    open: 'Buka',
    newTitle: 'Percakapan baru',
    mode: 'Mode',
    model: 'Model',
    comingLater: 'Segera hadir',
    rereadWarning: 'Dibaca ulang penuh',
    tokens: 'token',
    continue: 'Lanjutkan',
  },
  en: {
    newChat: 'New chat',
    search: 'Search',
    projects: 'Projects',
    artifacts: 'Artifacts',
    settings: 'Settings',
    recent: 'RECENT',
    collapse: 'Collapse sidebar',
    expand: 'Expand sidebar',
    modeChat: 'Chat Mode',
    rename: 'Rename',
    move: 'Move',
    openFolder: 'Open folder',
    save: 'Save',
    cancel: 'Cancel',
    noProject: 'No project',
    project: 'Project',
    startConversation: 'Start a conversation',
    askFirst: 'Ask your first question.',
    you: 'You',
    working: 'Working…',
    cache: 'cache',
    message: 'Message',
    send: 'Send',
    askAnything: 'Ask anything…',
    searchConversations: 'Search conversations',
    noResults: 'No results',
    noProjects: 'No projects yet',
    noArtifacts: 'No artifacts yet',
    theme: 'Theme',
    dark: 'Dark',
    light: 'Light',
    language: 'Language',
    indonesian: 'Indonesian',
    english: 'English',
    open: 'Open',
    newTitle: 'New chat',
    mode: 'Mode',
    model: 'Model',
    comingLater: 'Coming later',
    rereadWarning: 'Re-read at full price',
    tokens: 'tokens',
    continue: 'Continue',
  },
} as const;

export type StringKey = keyof (typeof strings)['id'];

function initialLocale(): Locale {
  if (typeof localStorage === 'undefined') return 'id';
  const saved = localStorage.getItem(STORAGE_KEY);
  return saved === 'en' ? 'en' : 'id';
}

/** Locale state and copy for every UI string. Indonesian is the default. */
class I18n {
  locale = $state<Locale>(initialLocale());

  t(key: StringKey): string {
    return strings[this.locale][key];
  }

  /** Locale-aware number formatting: 1.234 (id) vs 1,234 (en). */
  formatNumber(value: number): string {
    return new Intl.NumberFormat(this.locale === 'id' ? 'id-ID' : 'en-US').format(value);
  }

  set(locale: Locale): void {
    this.locale = locale;
    if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, locale);
    if (typeof document !== 'undefined') document.documentElement.lang = locale;
  }

  toggle(): void {
    this.set(this.locale === 'id' ? 'en' : 'id');
  }
}

export const i18n = new I18n();
