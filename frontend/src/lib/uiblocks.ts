import type { UIBlock, UIBlockSpec } from './types';

/** Normalize both a live ui.block payload and a persisted ui_block content part. */
export function parseUIBlock(raw: unknown): UIBlock | null {
  if (!raw || typeof raw !== 'object') return null;
  const value = raw as Record<string, unknown>;
  if (value.schemaVersion && value.schemaVersion !== 'sasmita.ui/v1') return null;
  const blockId = typeof value.blockId === 'string' ? value.blockId : '';
  if (!blockId) return null;
  let spec: UIBlockSpec | null = null;
  const candidate = value.spec;
  if (candidate && typeof candidate === 'object') spec = candidate as UIBlockSpec;
  const reason = typeof value.reason === 'string' ? value.reason : '';
  if (!spec && !reason) return null;
  return { blockId, spec, reason };
}

export function bindingKey(bind: string | undefined): string | null {
  if (!bind || !bind.startsWith('state.')) return null;
  const key = bind.slice('state.'.length);
  return key || null;
}

export function dataKey(ref: string | undefined): string | null {
  if (!ref || !ref.startsWith('data.')) return null;
  const key = ref.slice('data.'.length);
  return key || null;
}

export function withState(state: Record<string, unknown>, bind: string | undefined, value: unknown): Record<string, unknown> {
  const key = bindingKey(bind);
  if (!key) return state;
  return { ...state, [key]: value };
}

export function displayValue(value: unknown): string {
  if (typeof value === 'number') return new Intl.NumberFormat().format(value);
  if (value === null || value === undefined) return '';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

export function rowLabel(row: Record<string, unknown>): string {
  const value = row.label ?? row.name ?? row.kind ?? row.key ?? row.id;
  return displayValue(value);
}

export function rowValue(row: Record<string, unknown>): number {
  const raw = row.value ?? row.y ?? row.count;
  const num = typeof raw === 'number' ? raw : Number(raw);
  return Number.isFinite(num) ? num : 0;
}
