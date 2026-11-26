import { bind, isMyGo } from 'mygo-runtime';
import type { Locale } from './i18n.svelte';
import type { IPC } from './ipc';

/** A working environment, not a model choice (GLOSSARY.md). */
export interface Mode {
  id: string;
  name: string;
  instructions: string;
  toolsetIds: string[];
  skillIds: string[];
  workspace: string;
  permissionProfile: string;
  contextPolicy: string;
  available: boolean;
  phase: number;
}

export interface SwitchEstimate {
  estimatedTokens: number;
  warn: boolean;
  estimated: boolean;
}

export interface SwitchEstimateRequest {
  conversationId: string;
  modelRef: string;
  modeId: string;
}

export interface ModeIPC {
  listModes(): Promise<Mode[]>;
  estimateSwitch(req: SwitchEstimateRequest): Promise<SwitchEstimate>;
}

/** Matches modes.RereadWarningTokens (TAD §9.3). */
export const REREAD_WARNING_TOKENS = 4000;

import modeFixture from '../fixtures/modes.json';

/**
 * Deliberate seam-2 fake: Playwright has no Go process. It reads the same JSON
 * fixture as internal/modes/catalog_parity_test.go, so drift fails that test.
 * A model is selected per run, not as part of a Mode.
 */
export const BUILT_IN_MODES: Mode[] = modeFixture as Mode[];

const labels: Record<string, { id: string; en: string }> = {
  chat: { id: 'Chat', en: 'Chat' },
  riset: { id: 'Riset', en: 'Research' },
  penulisan: { id: 'Penulisan', en: 'Writing' },
  'analis-data': { id: 'Analis Data', en: 'Data analyst' },
  coding: { id: 'Coding', en: 'Coding' },
};

export function modeLabel(mode: Mode | undefined, locale: Locale): string {
  if (!mode) return 'Chat';
  return labels[mode.id]?.[locale] ?? mode.name;
}

const bindings = {
  listModes: bind<[], Mode[]>('shellService.ListModes', () => []),
  estimateSwitch: bind<[SwitchEstimateRequest], SwitchEstimate>(
    'shellService.EstimateSwitch',
    (req) => [req],
  ),
};

class MyGoModeIPC implements ModeIPC {
  listModes(): Promise<Mode[]> {
    return bindings.listModes();
  }

  estimateSwitch(req: SwitchEstimateRequest): Promise<SwitchEstimate> {
    return bindings.estimateSwitch(req);
  }
}

/** Conservative estimate matching modes.EstimateSwitch for seam 2. */
function estimateHistory(messages: { text: string }[]): SwitchEstimate {
  const chars = messages.reduce((sum, message) => sum + message.text.length + 16, 0);
  const estimatedTokens = Math.ceil(chars / 4);
  return { estimatedTokens, warn: estimatedTokens > REREAD_WARNING_TOKENS, estimated: true };
}

/** Fake IPC replays the same preflight through the conversation it already holds. */
class FakeModeIPC implements ModeIPC {
  constructor(private readonly ipc: IPC) {}

  async listModes(): Promise<Mode[]> {
    return structuredClone(BUILT_IN_MODES);
  }

  async estimateSwitch(req: SwitchEstimateRequest): Promise<SwitchEstimate> {
    const detail = await this.ipc.getConversation(req.conversationId);
    return estimateHistory(detail.messages);
  }
}

export function createModeIPC(ipc: IPC): ModeIPC {
  const params = new URLSearchParams(location.search);
  if (params.get('fake') === '1' || !isMyGo()) return new FakeModeIPC(ipc);
  return new MyGoModeIPC();
}
