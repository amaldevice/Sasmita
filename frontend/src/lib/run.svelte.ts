import type { RunEvent } from './types';

/**
 * Reduces the versioned RunEvent stream into the answer shown in the chat.
 * Seeded from the prototype's deterministic replay model (`prototype/ui-design`).
 */
export class ChatRun {
  runId = $state('');
  answer = $state('');
  streaming = $state(false);
  seq = $state(0);
  error = $state('');
  deltaCount = $state(0);
  usage = $state<{ inputTokens: number; outputTokens: number; cacheReadTokens: number } | null>(null);

  setRunId(id: string): void {
    this.runId = id;
  }

  begin(): void {
    this.streaming = true;
    this.answer = '';
    this.seq = 0;
    this.error = '';
    this.deltaCount = 0;
    this.usage = null;
  }

  apply(event: RunEvent): void {
    // Monotonic sequence guard: stale or duplicate events are ignored.
    if (event.seq > 0 && event.seq <= this.seq) return;
    this.seq = event.seq;
    const payload = event.payload as {
      version?: number;
      text?: string;
      message?: string;
      usage?: { inputTokens: number; outputTokens: number; cacheReadTokens: number };
    };
    switch (event.type) {
      case 'model.text_delta':
        this.answer += payload.text ?? '';
        this.deltaCount += 1;
        break;
      case 'model.usage':
        if (payload.usage) this.usage = payload.usage;
        break;
      case 'run.completed':
        this.streaming = false;
        break;
      case 'run.failed':
      case 'run.cancelled':
        this.streaming = false;
        this.error = payload.message ?? 'Run stopped';
        break;
    }
  }
}
