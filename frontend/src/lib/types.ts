export type Role = 'system' | 'user' | 'assistant';

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
}

export interface ConversationDetail {
  conversation: Conversation;
  messages: Message[];
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
  cancel: () => void;
}
