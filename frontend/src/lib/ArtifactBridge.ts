/**
 * Host side of artifact bridge v1. The iframe lives at an opaque origin, so
 * every message is checked against the exact iframe window and `origin===null`
 * before it is looked up in a fixed whitelist. Nothing here calls Go directly:
 * actions are delivered through the callbacks supplied by the host component.
 */
export const ARTIFACT_CHANNEL = 'sasmita.artifact';
export const BRIDGE_VERSION = 1;
export const MAX_STATE_BYTES = 32 * 1024;
export const MAX_DATA_BYTES = 5 * 1024 * 1024;

export const BRIDGE_TYPES = [
  'resize',
  'reportError',
  'saveState',
  'loadState',
  'getData',
  'suggestPrompt',
] as const;

export type BridgeType = (typeof BRIDGE_TYPES)[number];

export interface BridgeHandling {
  type: BridgeType | null;
  accepted: boolean;
  reason?: string;
}

/** Observable log for seam-2 tests and host diagnostics; bounded and inert. */
export const bridgeLog: BridgeHandling[] = [];
export function resetBridgeLog(): void {
  bridgeLog.length = 0;
}

export interface ArtifactBridgeOptions {
  frame: HTMLIFrameElement;
  /** Declared data refs, keyed by ref. Values must stay under MAX_DATA_BYTES. */
  data?: Record<string, unknown>;
  storage?: Storage;
  stateKey?: string;
  onResize?: (width: number, height: number) => void;
  onError?: (message: string) => void;
  onSaveState?: (bytes: number) => void;
  onSuggestPrompt?: (prompt: string) => void;
}

function byteLength(value: unknown): number {
  try {
    return new TextEncoder().encode(JSON.stringify(value) ?? '').length;
  } catch {
    return Number.POSITIVE_INFINITY;
  }
}

function record(result: BridgeHandling): BridgeHandling {
  bridgeLog.push(result);
  if (bridgeLog.length > 64) bridgeLog.shift();
  return result;
}

export class ArtifactBridge {
  private readonly options: ArtifactBridgeOptions;
  private readonly storage: Storage | undefined;
  private readonly stateKey: string;
  private listening = false;

  constructor(options: ArtifactBridgeOptions) {
    this.options = options;
    this.storage = options.storage;
    this.stateKey = options.stateKey ?? 'artifact.state.default';
  }

  start(): void {
    if (this.listening) return;
    window.addEventListener('message', this.listener);
    this.listening = true;
  }

  stop(): void {
    if (!this.listening) return;
    window.removeEventListener('message', this.listener);
    this.listening = false;
  }

  private readonly listener = (event: MessageEvent): void => {
    this.handle(event);
  };

  /** Returns a decision for every message; unknown or spoofed ones are inert. */
  handle(event: MessageEvent): BridgeHandling {
    const frameWindow = this.options.frame.contentWindow;
    if (event.origin !== 'null') return record({ type: null, accepted: false, reason: 'origin' });
    if (!frameWindow || event.source !== frameWindow) {
      return record({ type: null, accepted: false, reason: 'source' });
    }
    const message = event.data as Record<string, unknown> | null;
    if (!message || typeof message !== 'object' || Array.isArray(message)) {
      return record({ type: null, accepted: false, reason: 'shape' });
    }
    if (message.channel !== ARTIFACT_CHANNEL) {
      return record({ type: null, accepted: false, reason: 'channel' });
    }
    if (message.version !== BRIDGE_VERSION) {
      return record({ type: null, accepted: false, reason: 'version' });
    }
    const type = message.type as BridgeType;
    if (!BRIDGE_TYPES.includes(type)) {
      return record({ type: null, accepted: false, reason: 'type' });
    }
    const payload = (message.payload ?? {}) as Record<string, unknown>;

    switch (type) {
      case 'resize': {
        const { width, height } = payload;
        if (!isBoundedNumber(width) || !isBoundedNumber(height)) {
          return record({ type, accepted: false, reason: 'dimensions' });
        }
        this.options.onResize?.(width, height);
        break;
      }
      case 'reportError': {
        if (typeof payload.message !== 'string' || payload.message.length > 4096) {
          return record({ type, accepted: false, reason: 'message' });
        }
        this.options.onError?.(payload.message);
        break;
      }
      case 'saveState': {
        const bytes = byteLength(payload.state);
        if (bytes > MAX_STATE_BYTES) {
          return record({ type, accepted: false, reason: 'too_large' });
        }
        try {
          this.storage?.setItem(this.stateKey, JSON.stringify(payload.state ?? null));
        } catch {
          return record({ type, accepted: false, reason: 'storage' });
        }
        this.options.onSaveState?.(bytes);
        break;
      }
      case 'loadState': {
        let state: unknown = null;
        try {
          const raw = this.storage?.getItem(this.stateKey);
          if (raw) state = JSON.parse(raw);
        } catch {
          state = null;
        }
        const bytes = byteLength(state);
        if (bytes > MAX_STATE_BYTES) state = null;
        this.reply('state', { state });
        break;
      }
      case 'getData': {
        if (typeof payload.ref !== 'string' || !payload.ref) {
          return record({ type, accepted: false, reason: 'ref' });
        }
        const data = this.options.data?.[payload.ref];
        const bytes = byteLength(data);
        if (bytes > MAX_DATA_BYTES) {
          return record({ type, accepted: false, reason: 'too_large' });
        }
        this.reply('data', { data: data ?? null });
        break;
      }
      case 'suggestPrompt': {
        if (typeof payload.prompt !== 'string' || payload.prompt.length > 4096) {
          return record({ type, accepted: false, reason: 'prompt' });
        }
        this.options.onSuggestPrompt?.(payload.prompt);
        break;
      }
    }
    return record({ type, accepted: true });
  }

  /** Host → iframe replies. The frame is opaque, so targetOrigin must be '*'. */
  private reply(type: 'state' | 'data', payload: Record<string, unknown>): void {
    this.options.frame.contentWindow?.postMessage(
      { channel: ARTIFACT_CHANNEL, version: BRIDGE_VERSION, type, ...payload },
      '*',
    );
  }
}

/**
 * Host-side helpers for the off-screen lifecycle. They use the same key and
 * 32 KB cap as the bridge's saveState/loadState, so a parked artifact resumes
 * from exactly the state the iframe last reported.
 */
export function saveArtifactState(key: string, state: unknown): boolean {
  const bytes = byteLength(state);
  if (bytes > MAX_STATE_BYTES) {
    record({ type: 'saveState', accepted: false, reason: 'too_large' });
    return false;
  }
  try {
    localStorage.setItem(key, JSON.stringify(state ?? null));
  } catch {
    record({ type: 'saveState', accepted: false, reason: 'storage' });
    return false;
  }
  record({ type: 'saveState', accepted: true });
  return true;
}

export function loadArtifactState(key: string): unknown {
  let state: unknown = null;
  try {
    const raw = localStorage.getItem(key);
    if (raw) state = JSON.parse(raw);
  } catch {
    state = null;
  }
  if (byteLength(state) > MAX_STATE_BYTES) state = null;
  record({ type: 'loadState', accepted: true });
  return state;
}

function isBoundedNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 65536;
}
