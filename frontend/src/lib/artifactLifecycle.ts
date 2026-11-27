/**
 * Live artifact slot allocation, lifted from the `RenderFlow` module validated
 * on `prototype/genui-rendering-flow`. The sort order and slot counters are
 * kept verbatim so the prototype's scenarios stay the specification.
 */

export const MAX_LIVE = 2;
export const MAX_WEBGL = 1;

export type LifecyclePhase = 'ready' | 'streaming' | 'failed' | 'interrupted';

export type LifecycleView =
  | 'live'
  | 'parked'
  | 'slots_full'
  | 'froze_view'
  | 'in_window'
  | 'interrupted'
  | 'streaming'
  | 'failed';

export interface LifecycleBlock {
  id: string;
  title: string;
  summary: string;
  uri: string;
  tier: 2;
  phase: LifecyclePhase;
  version: number;
  webgl: boolean;
  visible: boolean;
  visibleAt: number;
  expanded: number;
  frozen: boolean;
  inWindow: boolean;
  userRunAt: number;
  view: LifecycleView;
}

export interface LifecycleConfig {
  maxLive: number;
  maxWebgl: number;
  autoRunVisible: boolean;
}

export interface LifecycleState {
  config: LifecycleConfig;
  blocks: LifecycleBlock[];
  tick: number;
}

export type LifecycleAction =
  | { type: 'SET_CONFIG'; patch: Partial<LifecycleConfig> }
  | { type: 'SCROLL_AWAY'; id: string }
  | { type: 'SCROLL_INTO_VIEW'; id: string }
  | { type: 'USER_RUN'; id: string }
  | { type: 'EXPAND'; id: string }
  | { type: 'COLLAPSE'; id: string }
  | { type: 'WATCHDOG_FREEZE'; id: string }
  | { type: 'OPEN_WINDOW'; id: string }
  | { type: 'CLOSE_WINDOW'; id: string }
  | { type: 'MODEL_EDIT'; id: string; version: number }
  | { type: 'MODEL_START_BLOCK'; id: string; title: string }
  | { type: 'MODEL_FINISH_BLOCK'; id: string }
  | { type: 'RELOAD_CONVERSATION' };

export function initial(blocks: LifecycleBlock[] = [], config: Partial<LifecycleConfig> = {}): LifecycleState {
  return {
    config: { maxLive: MAX_LIVE, maxWebgl: MAX_WEBGL, autoRunVisible: true, ...config },
    blocks,
    tick: 0,
  };
}

/** Allocate slots exactly as the prototype does; non-winners get a card view. */
export function allocate(s: LifecycleState): LifecycleState {
  const { maxLive, maxWebgl, autoRunVisible } = s.config;
  const eligible = s.blocks.filter(
    (b) => b.tier === 2 && b.phase === 'ready' && !b.frozen && !b.inWindow && b.visible
      && (autoRunVisible || b.userRunAt),
  );
  eligible.sort((a, b) => (b.expanded - a.expanded)
    || ((b.userRunAt || 0) - (a.userRunAt || 0))
    || (b.visibleAt - a.visibleAt));
  let live = 0;
  let webgl = 0;
  const winners = new Set<string>();
  for (const b of eligible) {
    if (live >= maxLive) break;
    if (b.webgl && webgl >= maxWebgl) continue;
    winners.add(b.id);
    live++;
    if (b.webgl) webgl++;
  }
  const blocks = s.blocks.map((b) => {
    if (b.phase !== 'ready') return b.view === b.phase ? b : { ...b, view: b.phase };
    let view: LifecycleView;
    if (b.inWindow) view = 'in_window';
    else if (b.frozen) view = 'froze_view';
    else if (winners.has(b.id)) view = 'live';
    else if (!b.visible) view = 'parked';
    else view = 'slots_full';
    return { ...b, view };
  });
  return { ...s, blocks };
}

function update(s: LifecycleState, id: string, patch: (b: LifecycleBlock) => Partial<LifecycleBlock>): LifecycleState {
  return { ...s, blocks: s.blocks.map((b) => (b.id === id ? { ...b, ...patch(b) } : b)) };
}

export function reduce(s0: LifecycleState, action: LifecycleAction): LifecycleState {
  const s = { ...s0, tick: s0.tick + 1 };
  const b = 'id' in action ? s.blocks.find((block) => block.id === action.id) : undefined;
  let next = s;
  switch (action.type) {
    case 'SET_CONFIG':
      next = { ...s, config: { ...s.config, ...action.patch } };
      break;
    case 'SCROLL_AWAY':
      if (b) next = update(s, b.id, () => ({ visible: false, expanded: 0 }));
      break;
    case 'SCROLL_INTO_VIEW':
      if (b) next = update(s, b.id, () => ({ visible: true, visibleAt: s.tick }));
      break;
    case 'USER_RUN':
      if (b) next = update(s, b.id, () => ({ userRunAt: s.tick, frozen: false }));
      break;
    case 'EXPAND':
      next = { ...s, blocks: s.blocks.map((x) => ({ ...x, expanded: x.id === action.id ? 1 : 0 })) };
      break;
    case 'COLLAPSE':
      if (b) next = update(s, b.id, () => ({ expanded: 0 }));
      break;
    case 'WATCHDOG_FREEZE':
      if (b) next = update(s, b.id, () => ({ frozen: true, userRunAt: 0, expanded: 0 }));
      break;
    case 'OPEN_WINDOW':
      if (b) next = update(s, b.id, () => ({ inWindow: true, expanded: 0 }));
      break;
    case 'CLOSE_WINDOW':
      if (b) next = update(s, b.id, () => ({ inWindow: false }));
      break;
    case 'MODEL_EDIT':
      if (b) next = update(s, b.id, () => ({ version: action.version }));
      break;
    case 'MODEL_START_BLOCK':
      next = {
        ...s,
        blocks: [...s.blocks, {
          id: action.id, title: action.title, summary: '', uri: `artifact://${action.id}/`,
          tier: 2, phase: 'streaming', version: 0, webgl: false, visible: true,
          visibleAt: s.tick, expanded: 0, frozen: false, inWindow: false,
          userRunAt: 0, view: 'streaming',
        }],
      };
      break;
    case 'MODEL_FINISH_BLOCK':
      if (b) next = update(s, b.id, () => ({ phase: 'ready', version: 1 }));
      break;
    case 'RELOAD_CONVERSATION':
      next = {
        ...s,
        blocks: s.blocks.map((x, i, arr) => ({
          ...x,
          userRunAt: 0,
          expanded: 0,
          inWindow: false,
          phase: x.phase === 'streaming' ? 'interrupted' : x.phase,
          visible: i >= arr.length - 2,
          visibleAt: i,
        })),
      };
      break;
  }
  return allocate(next);
}

export function lifecycleFixtures(): LifecycleBlock[] {
  const readys: Array<[string, string, string, number, boolean]> = [
    ['webgl-a', '3D orbit', 'Simulasi orbit 3D', 2, true],
    ['plain-b', 'Slider demo', 'Eksperimen kecepatan dengan slider', 3, false],
    ['webgl-c', 'Scene C', 'Adegan 3D lain', 1, true],
    ['plain-d', 'Notes D', 'Catatan percobaan', 1, false],
  ];
  const blocks: LifecycleBlock[] = readys.map(([id, title, summary, version, webgl], i) => ({
    id, title, summary, uri: `artifact://${id}/`, tier: 2, phase: 'ready', version, webgl,
    visible: true, visibleAt: i + 1, expanded: id === 'webgl-a' ? 1 : 0,
    frozen: false, inWindow: false, userRunAt: 0, view: 'live',
  }));
  // A block still streaming when the app closed becomes "interrupted" on reopen.
  blocks.unshift({
    id: 'stream-draft', title: 'Visual belum selesai', summary: '', uri: 'artifact://stream-draft/',
    tier: 2, phase: 'streaming', version: 0, webgl: false, visible: true, visibleAt: 0,
    expanded: 0, frozen: false, inWindow: false, userRunAt: 0, view: 'streaming',
  });
  return blocks;
}
