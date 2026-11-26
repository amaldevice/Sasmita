import type { UsageData } from './usage';

export type Role = 'system' | 'user' | 'assistant';

export type ShellView = 'chat' | 'baca' | 'search' | 'projects' | 'artifacts' | 'settings';

export interface Conversation {
  id: string;
  projectId: string;
  title: string;
  activeModeId: string;
  createdAt: string;
  updatedAt: string;
}

/** Tier 1 UI block (GLOSSARY): declarative spec rendered by Sasmita components. */
export interface UIComponent {
  id: string;
  type: 'chart' | 'table' | 'mermaid' | 'kpi' | 'math' | 'slider' | 'tabs' | 'toggle';
  label?: string;
  bind?: string;
  dataRef?: string;
  options?: string[];
  min?: number;
  max?: number;
  step?: number;
  source?: string;
  values?: Record<string, unknown>[];
  series?: number;
  rows?: number;
  columns?: number;
}

export interface UIBlockSpec {
  schemaVersion: string;
  blockId: string;
  kind?: string;
  title?: string;
  summary: string;
  layout?: { mode?: string; columns?: number };
  state?: Record<string, unknown>;
  data?: Record<string, { source?: string; encoding?: string; sha256?: string }>;
  components: UIComponent[];
}

export interface UIBlock {
  blockId: string;
  spec: UIBlockSpec | null;
  reason: string;
}

export interface UIData {
  ref: string;
  rows: Record<string, unknown>[];
  truncated: boolean;
}

export interface Message {
  id: string;
  conversationId: string;
  role: Role;
  text: string;
  blocks?: UIBlock[];
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
  modeId: string;
  providerId?: string;
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
  phase: 'proposed' | 'executed' | 'artifact' | 'failed' | 'denied';
  risk?: string;
  output?: string;
  summary?: string;
  artifactRef?: string;
  at: string;
}

/** One pending approval, sent as plain words from the policy engine. */
export interface Approval {
  approvalId: string;
  runId: string;
  callId: string;
  toolId: string;
  status: string;
  action: string;
  resource: string;
  impact: string;
  message: string;
  /** Exact bound arguments, echoed back on resolve for hash revalidation. */
  arguments?: string;
  argsHash: string;
  policyHash: string;
}

export type ApprovalDecision = 'allow_once' | 'always' | 'deny';

/** Sidebar signal for runs that need attention outside the open chat. */
export type SidebarBadge = 'waiting' | 'finished';

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
