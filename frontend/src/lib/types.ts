import type { UsageData } from './usage';

export type Role = 'system' | 'user' | 'assistant';

export type ShellView = 'chat' | 'search' | 'projects' | 'artifacts' | 'settings';

export interface Conversation {
  id: string;
  projectId: string;
  title: string;
  activeHarnessId: string;
  createdAt: string;
  updatedAt: string;
}

export interface Message {
  id: string;
  conversationId: string;
  role: Role;
  text: string;
  createdAt: string;
  /** Attached when the answer's run recorded usage; absent otherwise. */
  usage?: UsageData;
}

export interface ConversationDetail {
  conversation: Conversation;
  messages: Message[];
}

export interface Project {
  id: string;
  name: string;
  rootUri: string;
  createdAt: string;
  updatedAt: string;
}

export interface SearchResult {
  conversationId: string;
  title: string;
  projectId: string;
  snippet: string;
}

export interface StartRunRequest {
  conversationId: string;
  text: string;
  modelRef: string;
  harnessId: string;
}

// Mirrors domain.RunEvent in internal/domain/events.go. Tags are fixed by TAD §14.2.
export interface RunEvent {
  runId: string;
  seq: number;
  type: string;
  timestamp: string;
  payload: unknown;
}

export interface RunSnapshot {
  runId: string;
  status: string;
  answer: string;
  events: RunEvent[];
}

export interface RunHandle {
  runId: string;
}

/** One visible step in the activity timeline (ticket #22). */
export interface ToolStep {
  callId: string;
  name: string;
  phase: 'proposed' | 'executed' | 'artifact' | 'failed';
  risk?: string;
  output?: string;
  summary?: string;
  artifactRef?: string;
  at: string;
}

export type RunStatus =
  | 'queued'
  | 'preparing'
  | 'running'
  | 'awaiting_approval'
  | 'awaiting_user'
  | 'paused'
  | 'cancelling'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted';

/** UI-facing lifecycle record from desktop.RunDTO (TAD §8.3). */
export interface RunRecord {
  runId: string;
  conversationId: string;
  status: RunStatus | string;
  answer: string;
  originalRunId?: string;
}
