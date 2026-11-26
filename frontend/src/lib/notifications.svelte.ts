import { i18n } from './i18n.svelte';
import type { Approval } from './types';

/**
 * macOS notification hook. The shell adapter wires `__sasmitaNotify` to the
 * native notification bridge; this module only decides *when* to call it:
 * while the app is frontmost, the inline approval card is enough.
 * Real notification delivery is not verified by the fake-IPC seam.
 */
type Notifier = (title: string, body: string) => void;

function bridgeNotifier(): Notifier | null {
  if (typeof window === 'undefined') return null;
  const candidate = (window as unknown as Record<string, unknown>).__sasmitaNotify;
  return typeof candidate === 'function' ? (candidate as Notifier) : null;
}

export function notifyOnApproval(approval: Approval): void {
  const notify = bridgeNotifier();
  if (!notify) return;
  if (document.hidden || !document.hasFocus()) {
    notify(i18n.t('approvalNeeded'), `${approval.action} — ${approval.resource}`);
  }
}
