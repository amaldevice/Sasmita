import { bind, Channel, isMyGo } from 'mygo-runtime';
import approvalReplay from '../fixtures/approval-events.json';
import replay from '../fixtures/run-events.json';
import toolReplay from '../fixtures/tool-events.json';
import uiDataFixture from '../fixtures/ui-data.json';
import uiFallbackReplay from '../fixtures/ui-fallback.json';
import uiReplay from '../fixtures/ui-blocks.json';
import visualReplay from '../fixtures/visual-first.json';
import { shellService } from '../mygo';
import { isActiveStatus, latestSeq } from './activity';
import { usageFromEvents } from './usage';
import { parseUIBlock } from './uiblocks';
import type {
  Approval,
  ApprovalDecision,
  Conversation,
  ConversationDetail,
  Project,
  Role,
  RunEvent,
  SearchResult,
  RunHandle,
  RunRecord,
  RunSnapshot,
  StartRunRequest,
  UIData,
  UIBlock,
} from './types';

/** The typed IPC surface the Go shell (MyGo, or the Wails fallback) provides. */
export interface IPC {
  createConversation(projectId: string, title: string, modeId: string): Promise<Conversation>;
  listConversations(
    projectId: string,
    cursor: string,
    limit: number,
  ): Promise<{ items: Conversation[]; nextCursor: string }>;
  listRecentConversations(cursor: string, limit: number): Promise<{ items: Conversation[]; nextCursor: string }>;
  getConversation(conversationId: string): Promise<ConversationDetail>;
  renameConversation(conversationId: string, title: string): Promise<Conversation>;
  moveConversation(conversationId: string, projectId: string): Promise<Conversation>;
  searchConversations(query: string, limit: number): Promise<SearchResult[]>;
  listProjects(): Promise<Project[]>;
  /** Native file picker; returns filesystem paths usable as tool roots. */
  pickAttachments(): Promise<string[]>;
  startRun(req: StartRunRequest, onEvent: (event: RunEvent) => void): Promise<RunHandle>;
  cancelRun(runId: string): Promise<void>;
  listRuns(conversationId: string): Promise<RunRecord[]>;
  resumeRun(runId: string, onEvent: (event: RunEvent) => void): Promise<RunHandle>;
  getRunSnapshot(runId: string): Promise<RunSnapshot>;
  listPendingApprovals(conversationId: string): Promise<Approval[]>;
  /** args is the exact bound payload; an empty or changed value must be rejected. */
  resolveApproval(approvalId: string, decision: ApprovalDecision, args: string): Promise<void>;
  /** Bounded, read-only fetch of a large dataset referenced by a UI block. */
  getUIData(ref: string, limit: number): Promise<UIData>;
  /** Persist a committed, debounced view-local state patch as an append-only event. */
  saveUIState(runId: string, blockId: string, patch: Record<string, unknown>): Promise<void>;
}

// The generated client is refreshed by `mygo generate`; these two bindings use
// the same typed bridge for commands added in ticket #23.
const listPendingApprovalsBinding = bind<[string], Approval[]>(
  'shellService.ListPendingApprovals', (conversationID: string) => [conversationID],
);
const resolveApprovalBinding = bind<[string, string, string], void>(
  'shellService.ResolveApproval', (approvalID: string, decision: string, args: string) => [approvalID, decision, args],
);

/** Uses the TypeScript client generated from the Go bindings by `mygo generate`. */
export class MyGoIPC implements IPC {
  createConversation(projectId: string, title: string, modeId: string): Promise<Conversation> {
    return shellService.createConversation(projectId, title, modeId);
  }

  listConversations(
    projectId: string,
    cursor: string,
    limit: number,
  ): Promise<{ items: Conversation[]; nextCursor: string }> {
    return shellService.listConversations(projectId, cursor, limit);
  }

  listRecentConversations(cursor: string, limit: number): Promise<{ items: Conversation[]; nextCursor: string }> {
    return shellService.listRecentConversations(cursor, limit);
  }

  async getConversation(conversationId: string): Promise<ConversationDetail> {
    const detail = await shellService.getConversation(conversationId);
    return {
      conversation: detail.conversation,
      messages: detail.messages.map((m) => ({
        ...m,
        role: m.role as Role,
        blocks: (m.blocks ?? []).map(parseUIBlock).filter((block): block is UIBlock => block !== null),
      })),
    };
  }

  async startRun(req: StartRunRequest, onEvent: (event: RunEvent) => void): Promise<RunHandle> {
    const events = new Channel<RunEvent>((event) => onEvent(event));
    const runId = await shellService.startRun(req, events);
    return { runId };
  }

  async cancelRun(runId: string): Promise<void> {
    await shellService.cancelRun(runId);
  }

  async listRuns(conversationId: string): Promise<RunRecord[]> {
    return shellService.listRuns(conversationId);
  }

  async resumeRun(runId: string, onEvent: (event: RunEvent) => void): Promise<RunHandle> {
    const events = new Channel<RunEvent>((event) => onEvent(event));
    const resumedId = await shellService.resumeRun(runId, events);
    return { runId: resumedId };
  }

  renameConversation(conversationId: string, title: string): Promise<Conversation> {
    return shellService.renameConversation(conversationId, title);
  }

  moveConversation(conversationId: string, projectId: string): Promise<Conversation> {
    return shellService.moveConversation(conversationId, projectId);
  }

  async searchConversations(query: string, limit: number): Promise<SearchResult[]> {
    return shellService.searchConversations(query, limit);
  }

  listProjects(): Promise<Project[]> {
    return shellService.listProjects();
  }

  pickAttachments(): Promise<string[]> {
    return shellService.pickAttachments();
  }

  getRunSnapshot(runId: string): Promise<RunSnapshot> {
    return shellService.getRunSnapshot(runId);
  }

  listPendingApprovals(conversationId: string): Promise<Approval[]> {
    return listPendingApprovalsBinding(conversationId);
  }

  async resolveApproval(approvalId: string, decision: ApprovalDecision, args: string): Promise<void> {
    if (!args) throw new Error('approval arguments are missing');
    await resolveApprovalBinding(approvalId, decision, args);
  }

  async getUIData(ref: string, limit: number): Promise<UIData> {
    const data = await shellService.getUIData(ref, limit);
    return { ref: data.ref, rows: (data.rows ?? []) as UIData['rows'], truncated: !!data.truncated };
  }

  async saveUIState(runId: string, blockId: string, patch: Record<string, unknown>): Promise<void> {
    await shellService.saveUIState(runId, blockId, patch);
  }
}

const STORAGE_KEY = 'sasmita.fake.v1';

interface StoredApproval {
  runId: string;
  conversationId: string;
  decision: ApprovalDecision | null;
  card: Approval;
}

interface StoredState {
  conversations: Conversation[];
  details: Record<string, ConversationDetail>;
  snapshots: Record<string, RunSnapshot>;
  runs: RunRecord[];
  projects: Project[];
  approvals: Record<string, StoredApproval>;
  attachmentsByRun: Record<string, string[]>;
}

function seedState(): StoredState {
  const now = new Date().toISOString();
  const conversation: Conversation = {
    id: 'conv-1',
    projectId: 'p1',
    title: 'First chat',
    activeModeId: 'chat',
    createdAt: now,
    updatedAt: now,
  };
  return {
    conversations: [conversation],
    details: { [conversation.id]: { conversation, messages: [] } },
    snapshots: {},
    runs: [],
    projects: [
      { id: 'p1', name: 'Work', rootUri: 'file:///work', createdAt: now, updatedAt: now },
      { id: 'p2', name: 'Personal', rootUri: '', createdAt: now, updatedAt: now },
    ],
    approvals: {},
    attachmentsByRun: {},
  };
}

/**
 * Fake IPC for seam 2. It replays a recorded RunEvent JSON stream verbatim and
 * persists the resulting runs in localStorage so a reload behaves like an
 * application restart. Active runs left behind by a reload become interrupted,
 * mirroring the Go-side startup recovery.
 */
export class FakeIPC implements IPC {
  private state: StoredState;
  private controllers = new Map<string, { cancelled: boolean }>();
  private listeners = new Map<string, (event: RunEvent) => void>();
  private decisions = new Map<string, (decision: ApprovalDecision) => void>();
  private nextRun = 1;

  constructor() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      const parsed = raw ? (JSON.parse(raw) as Partial<StoredState>) : null;
      this.state = parsed
        ? {
            conversations: parsed.conversations ?? [],
            details: parsed.details ?? {},
            snapshots: parsed.snapshots ?? {},
            runs: parsed.runs ?? [],
            projects: parsed.projects ?? seedState().projects,
            approvals: parsed.approvals ?? {},
            attachmentsByRun: parsed.attachmentsByRun ?? {},
          }
        : seedState();
    } catch {
      this.state = seedState();
    }
  }

  private save(): void {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(this.state));
  }

  private newRunId(): string {
    return `run-${Date.now()}-${this.nextRun++}`;
  }

  async createConversation(projectId: string, title: string, modeId: string): Promise<Conversation> {
    const now = new Date().toISOString();
    const conversation: Conversation = {
      id: `conv-${Date.now()}`,
      projectId,
      title,
      activeModeId: modeId,
      createdAt: now,
      updatedAt: now,
    };
    this.state.conversations = [conversation, ...this.state.conversations];
    this.state.details[conversation.id] = { conversation, messages: [] };
    this.save();
    return conversation;
  }

  async listConversations(): Promise<{ items: Conversation[]; nextCursor: string }> {
    return { items: [...this.state.conversations], nextCursor: '' };
  }

  async getConversation(conversationId: string): Promise<ConversationDetail> {
    const detail = this.state.details[conversationId];
    if (!detail) throw new Error(`conversation ${conversationId} not found`);
    return structuredClone(detail);
  }

  async listRecentConversations(): Promise<{ items: Conversation[]; nextCursor: string }> {
    const items = [...this.state.conversations].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
    return { items, nextCursor: '' };
  }

  async renameConversation(conversationId: string, title: string): Promise<Conversation> {
    const now = new Date().toISOString();
    const update = (c: Conversation): Conversation => ({ ...c, title, updatedAt: now });
    this.state.conversations = this.state.conversations.map((c) => c.id === conversationId ? update(c) : c);
    const detail = this.state.details[conversationId];
    if (detail) this.state.details[conversationId] = { ...detail, conversation: update(detail.conversation) };
    this.save();
    return this.state.conversations.find((c) => c.id === conversationId)!;
  }

  async moveConversation(conversationId: string, projectId: string): Promise<Conversation> {
    const now = new Date().toISOString();
    const update = (c: Conversation): Conversation => ({ ...c, projectId, updatedAt: now });
    this.state.conversations = this.state.conversations.map((c) => c.id === conversationId ? update(c) : c);
    const detail = this.state.details[conversationId];
    if (detail) this.state.details[conversationId] = { ...detail, conversation: update(detail.conversation) };
    this.save();
    return this.state.conversations.find((c) => c.id === conversationId)!;
  }

  async searchConversations(query: string, limit: number): Promise<SearchResult[]> {
    const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (terms.length === 0) return [];
    const results: SearchResult[] = [];
    for (const c of this.state.conversations) {
      const messages = this.state.details[c.id]?.messages ?? [];
      for (const text of [c.title, ...messages.map((m) => m.text)]) {
        const lower = text.toLowerCase();
        if (terms.every((term) => lower.includes(term))) {
          results.push({ conversationId: c.id, title: c.title, projectId: c.projectId, snippet: text });
        }
      }
    }
    return results.slice(0, limit || 20);
  }

  async listProjects(): Promise<Project[]> {
    return structuredClone(this.state.projects);
  }

  async pickAttachments(): Promise<string[]> {
    return ['note.txt'];
  }

  async getRunSnapshot(runId: string): Promise<RunSnapshot> {
    const snapshot = this.state.snapshots[runId];
    if (!snapshot) throw new Error(`run ${runId} not found`);
    return structuredClone(snapshot);
  }

  async listPendingApprovals(conversationId: string): Promise<Approval[]> {
    return Object.values(this.state.approvals)
      .filter((stored) => stored.decision === null && stored.conversationId === conversationId)
      .map((stored) => structuredClone(stored.card));
  }

  async resolveApproval(approvalId: string, decision: ApprovalDecision, args: string): Promise<void> {
    const stored = this.state.approvals[approvalId];
    if (!stored) throw new Error(`approval ${approvalId} not found`);
    if (stored.decision !== null) return;
    if (!args || args !== (stored.card.arguments ?? '')) {
      throw new Error('approval arguments changed');
    }
    stored.decision = decision;
    this.save();
    const wake = this.decisions.get(stored.runId);
    if (wake) {
      this.decisions.delete(stored.runId);
      wake(decision);
    }
  }

  async getUIData(ref: string, limit: number): Promise<UIData> {
    if (!ref.startsWith('artifact://')) throw new Error('data reference is not an opaque artifact');
    const cap = limit > 0 && limit < 2000 ? limit : 2000;
    const all = uiDataFixture as UIData['rows'];
    const rows = all.slice(0, cap);
    return { ref, rows: structuredClone(rows), truncated: all.length > rows.length };
  }

  async saveUIState(runId: string, blockId: string, patch: Record<string, unknown>): Promise<void> {
    const snapshot = this.state.snapshots[runId];
    if (!snapshot) throw new Error(`run ${runId} not found`);
    snapshot.events.push({
      runId,
      seq: latestSeq(snapshot.events) + 1,
      type: 'ui.state',
      timestamp: new Date().toISOString(),
      payload: { version: 1, schemaVersion: 'sasmita.ui/v1', blockId, patch },
    });
    this.save();
  }

  async listRuns(conversationId: string): Promise<RunRecord[]> {
    let changed = false;
    for (const run of this.state.runs) {
      // A run waiting on an approval is paused, not stale; a reload must keep
      // its badge instead of turning it into an interrupted run.
      if (run.conversationId !== conversationId || !isActiveStatus(run.status) || run.status === 'awaiting_approval') continue;
      run.status = 'interrupted';
      const snapshot = this.state.snapshots[run.runId];
      if (snapshot) {
        snapshot.status = 'interrupted';
        snapshot.events.push({
          runId: run.runId,
          seq: latestSeq(snapshot.events) + 1,
          type: 'run.interrupted',
          timestamp: new Date().toISOString(),
          payload: { version: 1, status: 'interrupted' },
        });
      }
      changed = true;
    }
    if (changed) this.save();
    return this.state.runs
      .filter((run) => run.conversationId === conversationId)
      .map((run) => structuredClone(run));
  }

  async cancelRun(runId: string): Promise<void> {
    const controller = this.controllers.get(runId);
    if (controller) controller.cancelled = true;
    const snapshot = this.state.snapshots[runId];
    const record = this.state.runs.find((run) => run.runId === runId);
    if (!snapshot || !record || snapshot.status === 'cancelled') return;
    const event: RunEvent = {
      runId,
      seq: latestSeq(snapshot.events) + 1,
      type: 'run.cancelled',
      timestamp: new Date().toISOString(),
      payload: { version: 1, message: 'run cancelled' },
    };
    snapshot.events.push(event);
    snapshot.status = 'cancelled';
    record.status = 'cancelled';
    this.controllers.delete(runId);
    this.save();
    this.listeners.get(runId)?.(event);
  }

  async startRun(req: StartRunRequest, onEvent: (event: RunEvent) => void): Promise<RunHandle> {
    const runId = this.newRunId();
    const detail = this.state.details[req.conversationId];
    if (!detail) throw new Error(`conversation ${req.conversationId} not found`);

    detail.messages = [
      ...detail.messages,
      {
        id: `msg-user-${runId}`,
        conversationId: req.conversationId,
        role: 'user',
        text: req.text,
        createdAt: new Date().toISOString(),
      },
    ];
    this.state.runs.push({
      runId,
      conversationId: req.conversationId,
      status: 'running',
      answer: '',
    });
    this.state.attachmentsByRun[runId] = [...(req.attachments ?? [])];
    this.save();
    this.replay(runId, req.conversationId, onEvent);
    return { runId };
  }

  async resumeRun(runId: string, onEvent: (event: RunEvent) => void): Promise<RunHandle> {
    const original = this.state.runs.find((run) => run.runId === runId);
    if (!original || original.status !== 'interrupted') {
      throw new Error(`run ${runId} cannot be continued`);
    }
    const resumedId = this.newRunId();
    this.state.runs.push({
      runId: resumedId,
      conversationId: original.conversationId,
      status: 'running',
      answer: '',
      originalRunId: runId,
    });
    this.save();
    this.replay(resumedId, original.conversationId, onEvent);
    return { runId: resumedId };
  }

  private replay(runId: string, conversationId: string, onEvent: (event: RunEvent) => void): void {
    const detail = this.state.details[conversationId];
    const snapshot: RunSnapshot = { runId, status: 'running', answer: '', events: [] };
    this.state.snapshots[runId] = snapshot;
    const controller = { cancelled: false };
    this.controllers.set(runId, controller);
    this.listeners.set(runId, onEvent);
    let answer = '';
    const blocks: UIBlock[] = [];

    const params = new URLSearchParams(location.search);
    const stream = (params.get('visual') === '1'
      ? visualReplay
      : params.get('approval') === '1'
        ? approvalReplay
        : params.get('tools') === '1'
          ? toolReplay
          : params.get('ui') === 'fallback'
            ? uiFallbackReplay
            : params.get('ui') === '1'
              ? uiReplay
              : replay) as RunEvent[];
    void (async () => {
      for (const recorded of stream) {
        if (controller.cancelled) break;
        let event: RunEvent = {
          ...recorded,
          runId,
          payload: structuredClone(recorded.payload),
        };
        let waitForDecision = false;
        if (event.type === 'tool.approval_required') {
          const approvalId = (event.payload as { approvalId: string }).approvalId;
          const alreadyDecided = Object.values(this.state.approvals).some(
            (stored) => stored.runId === runId && stored.decision !== null,
          );
          if (!alreadyDecided) {
            this.state.approvals[approvalId] = {
              runId, conversationId, decision: null, card: event.payload as Approval,
            };
            const record = this.state.runs.find((run) => run.runId === runId);
            if (record) record.status = 'awaiting_approval';
            snapshot.status = 'awaiting_approval';
            this.save();
            waitForDecision = true;
          }
        }
        if (event.type === 'tool.executed' && this.state.approvals) {
          const denied = Object.values(this.state.approvals).some(
            (stored) => stored.runId === runId && stored.decision === 'deny',
          );
          if (denied) {
            event = {
              ...event,
              type: 'tool.denied',
              payload: { ...(event.payload as object), executed: false, status: 'denied', error: 'not approved' },
            };
          }
        }
        if (event.type === 'model.text_delta') {
          answer += (event.payload as { text: string }).text;
          snapshot.answer = answer;
        }
        if (event.type === 'ui.block') {
          const block = parseUIBlock(event.payload);
          if (block) blocks.push(block);
        }
        snapshot.events.push(event);
        if (event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.cancelled') {
          snapshot.status = event.type === 'run.completed' ? 'completed' : event.type === 'run.failed' ? 'failed' : 'cancelled';
          // Persist the assistant answer before delivering the terminal event,
          // so a refresh triggered by it sees the final message.
          detail.messages = [
            ...detail.messages,
            {
              id: `msg-assistant-${runId}`,
              conversationId,
              role: 'assistant',
              text: answer,
              blocks: blocks.length > 0 ? structuredClone(blocks) : undefined,
              createdAt: new Date().toISOString(),
              // Usage belongs to the answer, so a reopened conversation still
              // shows the same figures on hover (same data as the Go read path).
              usage: usageFromEvents(snapshot.events) ?? undefined,
            },
          ];
          const record = this.state.runs.find((run) => run.runId === runId);
          if (record) {
            record.status = snapshot.status;
            record.answer = answer;
          }
          this.controllers.delete(runId);
          this.save();
        }
        const stepParam = Number(params.get('step'));
        const stepMs = stepParam > 0 ? stepParam : params.get('slow') === '1' ? 220 : 16;
        await new Promise((resolve) => setTimeout(resolve, stepMs));
        if (!controller.cancelled) onEvent(event);
        if (waitForDecision && !controller.cancelled) {
          await new Promise<ApprovalDecision>((resolve) => this.decisions.set(runId, resolve));
        }
      }
      this.listeners.delete(runId);
    })();
  }
}

export function createIPC(): IPC {
  const params = new URLSearchParams(location.search);
  if (params.get('fake') === '1') return new FakeIPC();
  if (isMyGo()) return new MyGoIPC();
  // Dev fallback when opened in a plain browser without the shell bridge.
  return new FakeIPC();
}
