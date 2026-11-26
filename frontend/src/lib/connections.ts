import { bind } from 'mygo-runtime';

export type ProviderKind = 'openai-compatible' | 'anthropic-compatible';
export type CapabilityState = 'unknown' | 'supported' | 'unsupported';

export interface ProviderProfile {
  id: string;
  kind: ProviderKind;
  displayName: string;
  baseUrl: string;
  createdAt: string;
  updatedAt: string;
}

export interface Capabilities {
  tools: CapabilityState;
  vision: CapabilityState;
  contextWindow: number | null;
}

export interface ModelInfo {
  id: string;
  capabilities: Capabilities;
}

export interface ConnectionResult {
  ok: boolean;
  message: string;
}

export interface AddProfileInput {
  kind: ProviderKind;
  displayName: string;
  baseUrl: string;
  apiKey: string;
}

/** Typed bridge to the Go connection commands (mirrors desktop/providers.go). */
export interface ConnectionIPC {
  listProfiles(): Promise<ProviderProfile[]>;
  addProfile(input: AddProfileInput): Promise<ProviderProfile>;
  deleteProfile(id: string): Promise<void>;
  listModels(profileId: string): Promise<ModelInfo[]>;
  testConnection(profileId: string): Promise<ConnectionResult>;
}

const bindings = {
  listProfiles: bind<[], ProviderProfile[]>('shellService.ListProviderProfiles', () => []),
  addProfile: bind<[AddProfileInput], ProviderProfile>(
    'shellService.AddProviderProfile',
    (input) => [input],
  ),
  deleteProfile: bind<[string], void>('shellService.DeleteProviderProfile', (id) => [id]),
  listModels: bind<[string], ModelInfo[]>('shellService.ListModels', (id) => [id]),
  testConnection: bind<[string], ConnectionResult>(
    'shellService.TestConnection',
    (id) => [id],
  ),
};

export class MyGoConnectionIPC implements ConnectionIPC {
  listProfiles(): Promise<ProviderProfile[]> {
    return bindings.listProfiles();
  }
  addProfile(input: AddProfileInput): Promise<ProviderProfile> {
    return bindings.addProfile(input);
  }
  deleteProfile(id: string): Promise<void> {
    return bindings.deleteProfile(id);
  }
  listModels(profileId: string): Promise<ModelInfo[]> {
    return bindings.listModels(profileId);
  }
  testConnection(profileId: string): Promise<ConnectionResult> {
    return bindings.testConnection(profileId);
  }
}

const STORAGE_KEY = 'sasmita.connections.v1';

interface StoredState {
  profiles: ProviderProfile[];
  models: Record<string, ModelInfo[]>;
}

function baseline(): StoredState {
  const now = '2026-11-26T00:00:00.000Z';
  const make = (id: string, displayName: string, baseUrl: string): ProviderProfile => ({
    id,
    kind: 'anthropic-compatible',
    displayName,
    baseUrl,
    createdAt: now,
    updatedAt: now,
  });
  return {
    profiles: [
      make('profile-ok', 'Anthropic Fake', 'https://api.example.test'),
      make('profile-bad', 'Bad Key', 'https://bad.example.test'),
      make('profile-down', 'Offline', 'https://api.down.test'),
    ],
    models: {
      'profile-ok': [
        { id: 'model-a', capabilities: { tools: 'unknown', vision: 'unknown', contextWindow: null } },
        {
          id: 'model-b',
          capabilities: { tools: 'supported', vision: 'unsupported', contextWindow: 200000 },
        },
      ],
    },
  };
}

/** Fake IPC for seam 2. Keys are never stored here or rendered by the UI. */
export class FakeConnectionIPC implements ConnectionIPC {
  private state: StoredState;

  constructor() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      this.state = raw ? (JSON.parse(raw) as StoredState) : baseline();
    } catch {
      this.state = baseline();
    }
  }

  private save(): void {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(this.state));
  }

  async listProfiles(): Promise<ProviderProfile[]> {
    return structuredClone(this.state.profiles);
  }

  async addProfile(input: AddProfileInput): Promise<ProviderProfile> {
    // The key is deliberately dropped: only Go stores it in the keychain.
    const profile: ProviderProfile = {
      id: `profile-${Date.now()}`,
      kind: input.kind,
      displayName: input.displayName,
      baseUrl: input.baseUrl,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    this.state.profiles = [...this.state.profiles, profile];
    this.save();
    return profile;
  }

  async deleteProfile(id: string): Promise<void> {
    this.state.profiles = this.state.profiles.filter((p) => p.id !== id);
    this.save();
  }

  async listModels(profileId: string): Promise<ModelInfo[]> {
    return structuredClone(this.state.models[profileId] ?? []);
  }

  async testConnection(profileId: string): Promise<ConnectionResult> {
    if (profileId === 'profile-bad') {
      return {
        ok: false,
        message: 'The API key was rejected by Bad Key. Check the key and try again.',
      };
    }
    if (profileId === 'profile-down') {
      return {
        ok: false,
        message: 'Could not reach api.down.test. Check the base URL and your network.',
      };
    }
    return { ok: true, message: 'Connected.' };
  }
}

export function createConnectionIPC(): ConnectionIPC {
  const params = new URLSearchParams(location.search);
  if (params.get('fake') === '1') return new FakeConnectionIPC();
  return new MyGoConnectionIPC();
}
