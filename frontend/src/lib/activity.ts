import { i18n } from './i18n.svelte';
import type { RunEvent, RunRecord, RunSnapshot } from './types';

/** TAD §8.3 states that still represent in-flight work. */
export const ACTIVE_STATUSES = [
  'queued',
  'preparing',
  'running',
  'awaiting_approval',
  'awaiting_user',
  'paused',
  'cancelling',
] as const;

/** Terminal TAD states plus the recovery state need no further UI activity. */
export const SETTLED_STATUSES = ['completed', 'failed', 'cancelled', 'interrupted'] as const;

export function isActiveStatus(status: string): boolean {
  return (ACTIVE_STATUSES as readonly string[]).includes(status);
}

export function canContinue(run: RunRecord | null | undefined): boolean {
  return run?.status === 'interrupted';
}

export function activityLabel(status: string): string {
  switch (status) {
    case 'queued':
      return i18n.t('statusQueued');
    case 'preparing':
      return i18n.t('statusPreparing');
    case 'cancelling':
      return i18n.t('statusCancelling');
    case 'awaiting_approval':
      return i18n.t('statusAwaitingApproval');
    case 'awaiting_user':
      return i18n.t('statusAwaitingUser');
    case 'paused':
      return i18n.t('statusPaused');
    default:
      return i18n.t('working');
  }
}

/** Highest monotonic seq in a recorded stream; 0 for an empty stream. */
export function latestSeq(events: RunEvent[]): number {
  return events.reduce((max, event) => Math.max(max, event.seq), 0);
}

/** Rebuild the UI-visible answer from persisted deltas after reconnect. */
export function answerFromSnapshot(snapshot: RunSnapshot): string {
  if (snapshot.answer) return snapshot.answer;
  let answer = '';
  for (const event of snapshot.events) {
    if (event.type !== 'model.text_delta') continue;
    const payload = event.payload as { text?: string } | null;
    answer += payload?.text ?? '';
  }
  return answer;
}
