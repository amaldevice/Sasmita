/** UI-facing artifact records (TAD §13.3 + Tier 2 fields). */
export interface ArtifactVersionRecord {
  version: number;
  ref: string;
  sha256: string;
  sizeBytes: number;
  sourceKind: 'html' | 'jsx';
}

export interface ArtifactRecord {
  id: string;
  ref: string;
  uri: string;
  title: string;
  sourceKind: 'html' | 'jsx';
  runtimeVersion: string;
  status: 'ok' | 'froze_view';
  versions: ArtifactVersionRecord[];
}

export interface ArtifactIPC {
  listArtifacts(): Promise<ArtifactRecord[]>;
  openArtifact(id: string): Promise<ArtifactRecord | null>;
}

// Seam-2 fixture: replayed exactly like the RunEvent fixtures in ipc.ts.
export const fixtureArtifacts: ArtifactRecord[] = [
  {
    id: 'abc123',
    ref: 'artifact://abc123',
    uri: 'artifact://abc123/',
    title: 'Neural network explainer',
    sourceKind: 'jsx',
    runtimeVersion: 'artifact-runtime/1',
    status: 'ok',
    versions: [
      { version: 1, ref: 'artifact://abc123#v1', sha256: 'a1b2c3d4e5f6', sizeBytes: 2048, sourceKind: 'jsx' },
      { version: 2, ref: 'artifact://abc123#v2', sha256: 'f6e5d4c3b2a1', sizeBytes: 2176, sourceKind: 'jsx' },
    ],
  },
  {
    id: 'def456',
    ref: 'artifact://def456',
    uri: 'artifact://def456/',
    title: 'Chart draft',
    sourceKind: 'html',
    runtimeVersion: 'artifact-runtime/1',
    status: 'froze_view',
    versions: [{ version: 1, ref: 'artifact://def456#v1', sha256: '0123456789ab', sizeBytes: 512, sourceKind: 'html' }],
  },
];

/** Declared data refs the host resolves for getData(ref) (Tier 2 manifest). */
export const fixtureArtifactData: Record<string, unknown> = {
  'artifact://data/points': [{ x: 1, y: 2 }, { x: 2, y: 3 }],
};

/** An artifact whose source is still streaming and has not been stored yet. */
export const pendingArtifact: ArtifactRecord = {
  id: 'pending',
  ref: '',
  uri: '',
  title: 'Sedang ditulis / Being written',
  sourceKind: 'jsx',
  runtimeVersion: 'artifact-runtime/1',
  status: 'ok',
  versions: [],
};

export function createArtifactIPC(): ArtifactIPC {
  return {
    async listArtifacts() {
      return structuredClone(fixtureArtifacts);
    },
    async openArtifact(id: string) {
      return structuredClone(fixtureArtifacts.find((a) => a.id === id) ?? null);
    },
  };
}
