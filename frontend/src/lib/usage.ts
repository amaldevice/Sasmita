import { bind, isMyGo } from 'mygo-runtime';
import recordedUsage from '../fixtures/diagnostics-usage.json';
import type { Locale } from './i18n.svelte';
import type { RunEvent } from './types';

/** Per-answer read model. Mirrors desktop.UsageDTO / internal/usage.AnswerUsage. */
export interface UsageData {
  providerId: string;
  modelId: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  uncachedTokens: number;
  tokenEstimated: boolean;
  hitRate: number;
  costUsd: number;
  costEstimated: boolean;
  durationMs: number;
}

export interface UsageRow {
  key: string;
  providerId: string;
  requests: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  hitRate: number;
  costUsd: number;
  costEstimated: boolean;
}

export interface UsageDrop {
  day: string;
  from: number;
  to: number;
}

export interface UsageReport {
  models: UsageRow[];
  days: UsageRow[];
  drop: UsageDrop | null;
}

/** Smallest fall between consecutive days worth warning about. */
export const HIT_RATE_DROP_THRESHOLD = 0.1;

// Conservative application-side rates in USD per million tokens. A cost that
// comes from these is always labelled an estimate, never provider-reported.
// Mirrors internal/usage/cost.go.
interface Rate {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
}
const DEFAULT_RATE: Rate = { input: 0.15, output: 0.6, cacheRead: 0.0375, cacheWrite: 0.1875 };
const MODEL_RATES: Record<string, Rate> = {
  'gpt-4o': { input: 2.5, output: 10, cacheRead: 1.25, cacheWrite: 3.125 },
  'gpt-4o-mini': DEFAULT_RATE,
  'claude-3-5-sonnet': { input: 3, output: 15, cacheRead: 0.3, cacheWrite: 3.75 },
  'claude-3-5-haiku': { input: 0.8, output: 4, cacheRead: 0.08, cacheWrite: 1 },
  'claude-3-7-sonnet': { input: 3, output: 15, cacheRead: 0.3, cacheWrite: 3.75 },
};

interface RawUsage {
  providerId?: string;
  modelId?: string;
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  uncachedTokens?: number;
  estimated?: boolean;
  costEstimate?: string;
  costUsd?: number;
}

export function hitRate(input: number, cacheRead: number): number {
  return input > 0 ? cacheRead / input : 0;
}

function estimateCost(modelId: string, uncached: number, cacheRead: number, cacheWrite: number, output: number): number {
  const rate = MODEL_RATES[modelId] ?? DEFAULT_RATE;
  return (uncached * rate.input + cacheRead * rate.cacheRead + cacheWrite * rate.cacheWrite + output * rate.output) / 1_000_000;
}

/** Normalize a recorded usage payload, reusing #17's cache-field semantics. */
function normalize(raw: RawUsage, durationMs = 0): UsageData | null {
  if (!raw) return null;
  let input = raw.inputTokens ?? 0;
  const output = raw.outputTokens ?? 0;
  const cacheRead = raw.cacheReadTokens ?? 0;
  const cacheWrite = raw.cacheWriteTokens ?? 0;
  let uncached = raw.uncachedTokens ?? 0;
  let tokenEstimated = raw.estimated === true;
  if (uncached <= 0 && input > 0) {
    uncached = input - cacheRead - cacheWrite;
    if (uncached < 0) {
      uncached = 0;
      tokenEstimated = true;
    }
  }
  if (input === 0 && (cacheRead > 0 || cacheWrite > 0 || uncached > 0)) {
    input = cacheRead + cacheWrite + uncached;
    tokenEstimated = true;
  }
  if (input <= 0 && output <= 0 && cacheRead <= 0) return null;

  const reported = raw.costUsd ?? (raw.costEstimate ? Number.parseFloat(raw.costEstimate) : NaN);
  const providerReported = Number.isFinite(reported) && reported > 0;
  return {
    providerId: raw.providerId ?? '',
    modelId: raw.modelId ?? '',
    inputTokens: input,
    outputTokens: output,
    cacheReadTokens: cacheRead,
    cacheWriteTokens: cacheWrite,
    uncachedTokens: uncached,
    tokenEstimated,
    hitRate: hitRate(input, cacheRead),
    costUsd: providerReported ? reported : estimateCost(raw.modelId ?? '', uncached, cacheRead, cacheWrite, output),
    costEstimated: !providerReported,
    durationMs,
  };
}

function usagePayload(event: RunEvent): RawUsage | null {
  if (event.type !== 'model.usage') return null;
  const payload = event.payload as { usage?: RawUsage } | null;
  return payload?.usage ?? null;
}

/** Usage for a single recorded event, e.g. the live hover popover. */
export function usageFromEvent(event: RunEvent): UsageData | null {
  const raw = usagePayload(event);
  return raw ? normalize(raw) : null;
}

/** Usage for a whole recorded stream, with duration from run start. */
export function usageFromEvents(events: RunEvent[]): UsageData | null {
  const usageEvent = [...events].reverse().find((event) => event.type === 'model.usage');
  const raw = usageEvent ? usagePayload(usageEvent) : null;
  if (!raw) return null;
  const started = events.find((event) => event.type === 'run.queued' || event.type === 'run.started');
  const durationMs = started && usageEvent
    ? Math.max(0, Date.parse(usageEvent.timestamp) - Date.parse(started.timestamp))
    : 0;
  return normalize(raw, durationMs);
}

function numberFormat(locale: Locale): Intl.NumberFormat {
  return new Intl.NumberFormat(locale === 'id' ? 'id-ID' : 'en-US');
}

export function formatCount(value: number, locale: Locale): string {
  return numberFormat(locale).format(value);
}

export function formatPercent(rate: number, locale: Locale): string {
  return `${numberFormat(locale).format(Math.round(rate * 1000) / 10)}%`;
}

export function formatCost(value: UsageData | number, locale: Locale): string {
  const amount = typeof value === 'number' ? value : value.costUsd;
  return new Intl.NumberFormat(locale === 'id' ? 'id-ID' : 'en-US', {
    style: 'currency', currency: 'USD', minimumFractionDigits: 6, maximumFractionDigits: 6,
  }).format(amount);
}

/** DESIGN.md: `waktu ↑ input ↓ output % cache`, monospace data only. */
export function formatUsageLine(usage: UsageData, locale: Locale): string {
  const waktu = usage.durationMs > 0
    ? `${numberFormat(locale).format(Math.round(usage.durationMs / 100) / 10)}s`
    : '—';
  return `${waktu} ↑ ${formatCount(usage.inputTokens, locale)} ↓ ${formatCount(usage.outputTokens, locale)} ${formatPercent(usage.hitRate, locale)} cache`;
}

interface StoredUsage {
  day: string;
  runId: string;
  seq: number;
  usage: UsageData;
}

const STORAGE_KEY = 'sasmita.usage.v1';

function stored(): StoredUsage[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    const parsed = raw ? (JSON.parse(raw) as StoredUsage[]) : [];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/** Record a live recorded-event usage figure for the Diagnostics seam. */
export function recordUsageEvent(event: RunEvent): void {
  const usage = usageFromEvent(event);
  if (!usage) return;
  const records = stored();
  if (records.some((record) => record.runId === event.runId && record.seq === event.seq)) return;
  records.push({ day: event.timestamp.slice(0, 10), runId: event.runId, seq: event.seq, usage });
  localStorage.setItem(STORAGE_KEY, JSON.stringify(records));
}

function addRow(rows: Map<string, UsageRow>, key: string, usage: UsageData): void {
  let row = rows.get(key);
  if (!row) {
    row = {
      key, providerId: usage.providerId, requests: 0, inputTokens: 0, outputTokens: 0,
      cacheReadTokens: 0, hitRate: 0, costUsd: 0, costEstimated: false,
    };
    rows.set(key, row);
  }
  row.requests += 1;
  row.inputTokens += usage.inputTokens;
  row.outputTokens += usage.outputTokens;
  row.cacheReadTokens += usage.cacheReadTokens;
  row.costUsd += usage.costUsd;
  if (usage.costEstimated) row.costEstimated = true;
}

function accumulate(records: Iterable<StoredUsage>, models: Map<string, UsageRow>, days: Map<string, UsageRow>): void {
  for (const record of records) {
    addRow(models, record.usage.modelId || 'unknown', record.usage);
    addRow(days, record.day, record.usage);
  }
}

function finish(rows: Map<string, UsageRow>): UsageRow[] {
  const out = [...rows.values()];
  for (const row of out) row.hitRate = hitRate(row.inputTokens, row.cacheReadTokens);
  return out.sort((a, b) => a.key.localeCompare(b.key));
}

/** Pure aggregation behind Settings › Diagnostics. */
export function aggregateRecords(records: StoredUsage[]): UsageReport {
  const models = new Map<string, UsageRow>();
  const days = new Map<string, UsageRow>();
  accumulate(records, models, days);
  return reportFrom(models, days);
}

function reportFrom(models: Map<string, UsageRow>, days: Map<string, UsageRow>): UsageReport {
  const modelRows = finish(models);
  const dayRows = [...days.values()];
  for (const row of dayRows) row.hitRate = hitRate(row.inputTokens, row.cacheReadTokens);
  dayRows.sort((a, b) => a.key.localeCompare(b.key));
  let drop: UsageDrop | null = null;
  for (let i = 1; i < dayRows.length; i += 1) {
    const previous = dayRows[i - 1].hitRate;
    const current = dayRows[i].hitRate;
    if (current < previous - HIT_RATE_DROP_THRESHOLD) {
      drop = { day: dayRows[i].key, from: previous, to: current };
      break;
    }
  }
  return { models: modelRows, days: dayRows, drop };
}

/** Typed binding to the Go Diagnostics command. */
export interface UsageIPC {
  getUsageReport(): Promise<UsageReport>;
}

const bindings = {
  getUsageReport: bind<[], UsageReport>('shellService.GetUsageReport', () => []),
};

export class MyGoUsageIPC implements UsageIPC {
  getUsageReport(): Promise<UsageReport> {
    return bindings.getUsageReport();
  }
}

/** Replays recorded model.usage events plus live ones from localStorage. */
export class FakeUsageIPC implements UsageIPC {
  async getUsageReport(): Promise<UsageReport> {
    const models = new Map<string, UsageRow>();
    const days = new Map<string, UsageRow>();
    const all = new Map<string, StoredUsage>();
    for (const event of recordedUsage as RunEvent[]) {
      const usage = usageFromEvent(event);
      if (!usage) continue;
      all.set(`${event.runId}:${event.seq}`, {
        day: event.timestamp.slice(0, 10), runId: event.runId, seq: event.seq, usage,
      });
    }
    for (const record of stored()) all.set(`${record.runId}:${record.seq}`, record);

    accumulate(all.values(), models, days);
    return reportFrom(models, days);
  }
}

export function createUsageIPC(): UsageIPC {
  const params = new URLSearchParams(location.search);
  if (params.get('fake') === '1' || !isMyGo()) return new FakeUsageIPC();
  return new MyGoUsageIPC();
}
