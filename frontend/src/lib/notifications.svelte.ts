import { i18n } from './i18n.svelte';
import type { Approval } from './types';

/**
 * macOS notification hook. In the real shell the notifier is the typed MyGo
 * binding installed at startup; the Go side checks `NotificationsSupported`
 * and frontmost state before showing a native notification. The fake-IPC seam
 * injects a browser notifier to assert only the focus gate.
 */
type Notifier = (title: string, body: string) => void;

let shellNotifier: Notifier | null = null;

export function setNotifier(notifier: Notifier | null): void {
  shellNotifier = notifier;
}

function bridgeNotifier(): Notifier | null {
  if (shellNotifier) return shellNotifier;
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
