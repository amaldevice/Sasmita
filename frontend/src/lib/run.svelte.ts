import { answerFromSnapshot } from './activity';
import type { Approval, RunEvent, RunSnapshot, ToolStep } from './types';

/**
 * Reduces the versioned RunEvent stream into the answer shown in the chat.
 * Seeded from the prototype's deterministic replay model (`prototype/ui-design`).
 */
export class ChatRun {
  runId = $state('');
  answer = $state('');
  streaming = $state(false);
  status = $state('');
  cancelling = $state(false);
  seq = $state(0);
  error = $state('');
  deltaCount = $state(0);
  usage = $state<{ inputTokens: number; outputTokens: number; cacheReadTokens: number } | null>(null);
  steps = $state<ToolStep[]>([]);
  approval = $state<Approval | null>(null);
  startedAt = $state('');
  lastEventAt = $state('');

  /** Elapsed seconds between the first and most recent recorded event. */
  elapsedSeconds = $derived.by(() => {
    if (!this.startedAt || !this.lastEventAt) return 0;
    const span = Date.parse(this.lastEventAt) - Date.parse(this.startedAt);
    return Number.isFinite(span) ? Math.max(0, Math.floor(span / 1000)) : 0;
  });

  setRunId(id: string): void {
    this.runId = id;
  }

  /** Stops the pulse as soon as the user answers, before the next event. */
  clearApproval(): void {
    this.approval = null;
  }

  begin(): void {
    this.streaming = true;
    this.status = 'queued';
    this.cancelling = false;
    this.answer = '';
    this.seq = 0;
    this.error = '';
    this.deltaCount = 0;
    this.usage = null;
    this.steps = [];
    this.approval = null;
    this.startedAt = '';
    this.lastEventAt = '';
  }

  /** Rebuild stream-derived UI state from GetRunSnapshot after reconnect. */
  restore(snapshot: RunSnapshot): void {
    this.begin();
    this.runId = snapshot.runId;
    for (const event of snapshot.events) this.apply(event);
    this.answer = answerFromSnapshot(snapshot);
    this.status = snapshot.status;
    this.streaming = false;
    this.cancelling = false;
  }

  apply(event: RunEvent): void {
    // Monotonic sequence guard: stale or duplicate events are ignored.
    if (event.seq > 0 && event.seq <= this.seq) return;
    this.seq = event.seq;
    if (!this.startedAt && event.timestamp) this.startedAt = event.timestamp;
    if (event.timestamp) this.lastEventAt = event.timestamp;
    const payload = event.payload as {
      version?: number;
      text?: string;
      message?: string;
      usage?: { inputTokens: number; outputTokens: number; cacheReadTokens: number };
      callId?: string;
      name?: string;
      risk?: string;
      output?: string;
      summary?: string;
      artifactRef?: string;
    };
    switch (event.type) {
      case 'run.queued':
        this.status = 'queued';
        break;
      case 'run.preparing':
        this.status = 'preparing';
        break;
      case 'run.started':
        this.status = 'running';
        this.streaming = true;
        break;
      case 'run.cancelling':
        this.status = 'cancelling';
        this.cancelling = true;
        break;
      case 'tool.approval_required':
        this.status = 'awaiting_approval';
        this.approval = payload as unknown as Approval;
        break;
      case 'tool.proposed':
      case 'tool.executed':
      case 'tool.artifact':
      case 'tool.failed':
      case 'tool.denied': {
        if (event.type === 'tool.executed' || event.type === 'tool.denied') this.approval = null;
        const phase = event.type.slice('tool.'.length) as ToolStep['phase'];
        this.steps = [
          ...this.steps,
          {
            callId: payload.callId ?? '',
            name: payload.name ?? '',
            phase,
            risk: payload.risk,
            output: payload.output,
            summary: payload.summary,
            artifactRef: payload.artifactRef,
            at: event.timestamp,
          },
        ];
        break;
      }
      case 'run.interrupted':
        this.status = 'interrupted';
        this.streaming = false;
        break;
      case 'model.text_delta':
        this.answer += payload.text ?? '';
        this.deltaCount += 1;
        break;
      case 'model.usage':
        if (payload.usage) this.usage = payload.usage;
        break;
      case 'run.completed':
        this.status = 'completed';
        this.streaming = false;
        this.approval = null;
        break;
      case 'run.failed':
        this.status = 'failed';
        this.streaming = false;
        this.approval = null;
        this.error = payload.message ?? 'Run stopped';
        break;
      case 'run.cancelled':
        this.status = 'cancelled';
        this.streaming = false;
        this.approval = null;
        this.cancelling = false;
        this.error = payload.message ?? 'Run dibatalkan';
        break;
    }
  }
}
