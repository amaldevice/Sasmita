import { Channel, isMyGo } from 'mygo-runtime';
import replay from '../fixtures/run-events.json';
import toolReplay from '../fixtures/tool-events.json';
import { shellService } from '../mygo';
import { isActiveStatus, latestSeq } from './activity';
import type {
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
} from './types';

/** The typed IPC surface the Go shell (MyGo, or the Wails fallback) provides. */
export interface IPC {
  createConversation(projectId: string, title: string, harnessId: string): Promise<Conversation>;
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
  startRun(req: StartRunRequest, onEvent: (event: RunEvent) => void): Promise<RunHandle>;
  cancelRun(runId: string): Promise<void>;
  listRuns(conversationId: string): Promise<RunRecord[]>;
  resumeRun(runId: string, onEvent: (event: RunEvent) => void): Promise<RunHandle>;
  getRunSnapshot(runId: string): Promise<RunSnapshot>;
}

/** Uses the TypeScript client generated from the Go bindings by `mygo generate`. */
export class MyGoIPC implements IPC {
  createConversation(projectId: string, title: string, harnessId: string): Promise<Conversation> {
    return shellService.createConversation(projectId, title, harnessId);
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
      messages: detail.messages.map((m) => ({ ...m, role: m.role as Role })),
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

  getRunSnapshot(runId: string): Promise<RunSnapshot> {
    return shellService.getRunSnapshot(runId);
  }
}

const STORAGE_KEY = 'sasmita.fake.v1';

interface StoredState {
  conversations: Conversation[];
  details: Record<string, ConversationDetail>;
  snapshots: Record<string, RunSnapshot>;
  runs: RunRecord[];
  projects: Project[];
}

function seedState(): StoredState {
  const now = new Date().toISOString();
  const conversation: Conversation = {
    id: 'conv-1',
    projectId: 'p1',
    title: 'First chat',
    activeHarnessId: 'chat',
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

  async createConversation(projectId: string, title: string, harnessId: string): Promise<Conversation> {
    const now = new Date().toISOString();
    const conversation: Conversation = {
      id: `conv-${Date.now()}`,
      projectId,
      title,
      activeHarnessId: harnessId,
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

  async getRunSnapshot(runId: string): Promise<RunSnapshot> {
    const snapshot = this.state.snapshots[runId];
    if (!snapshot) throw new Error(`run ${runId} not found`);
    return structuredClone(snapshot);
  }

  async listRuns(conversationId: string): Promise<RunRecord[]> {
    let changed = false;
    for (const run of this.state.runs) {
      if (run.conversationId !== conversationId || !isActiveStatus(run.status)) continue;
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

    const params = new URLSearchParams(location.search);
    const stream = (params.get('tools') === '1' ? toolReplay : replay) as RunEvent[];
    void (async () => {
      for (const recorded of stream) {
        if (controller.cancelled) break;
        const event: RunEvent = {
          ...recorded,
          runId,
          payload: structuredClone(recorded.payload),
        };
        if (event.type === 'model.text_delta') {
          answer += (event.payload as { text: string }).text;
          snapshot.answer = answer;
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
              createdAt: new Date().toISOString(),
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
