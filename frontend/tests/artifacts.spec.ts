import { expect, test, type Page } from '@playwright/test';

/**
 * Seam 2: the artifact library, the sandbox-only iframe, and bridge v1.
 * Messages are delivered with the exact source/origin the host checks. The
 * "messages never reach Go" half of isolation is a packaged-app (WKWebView)
 * check recorded in docs/benchmarks, not claimed as verified here.
 */

interface BridgeDecision {
  type: string | null;
  accepted: boolean;
  reason?: string;
}

function msg(type: string, payload: unknown) {
  return { channel: 'sasmita.artifact', version: 1, type, payload };
}

async function post(
  page: Page,
  data: unknown,
  options: { origin?: string; source?: 'frame' | 'parent' } = {},
): Promise<void> {
  await page.evaluate(
    ({ data, origin, source }) => {
      const frame = document.querySelector('[data-testid="artifact-iframe"]') as HTMLIFrameElement;
      const from = source === 'parent' ? window : frame.contentWindow;
      window.dispatchEvent(
        new MessageEvent('message', { data, origin, source: from as MessageEventSource | null }),
      );
    },
    { data, origin: options.origin ?? 'null', source: options.source ?? 'frame' },
  );
}

async function lastDecision(page: Page): Promise<BridgeDecision | undefined> {
  return page.evaluate(() => {
    const log = (window as unknown as { __artifactBridgeLog?: BridgeDecision[] }).__artifactBridgeLog ?? [];
    return log[log.length - 1];
  });
}

async function openLibrary(page: Page): Promise<void> {
  await page.getByTestId('nav-artifacts').click();
  await expect(page.getByTestId('artifacts-library')).toBeVisible();
  await expect(page.getByTestId('artifact-iframe').first()).toBeVisible();
}

test('library lists every artifact with its immutable versions and runtime', async ({ page }) => {
  await page.goto('/?fake=1');
  await openLibrary(page);
  await expect(page.getByTestId('artifact-row')).toHaveCount(2);
  await expect(page.getByTestId('artifact-ref').first()).toHaveText('artifact://abc123');
  await expect(page.getByTestId('artifact-versions').first()).toContainText(/2 (versi|versions)/);
  await expect(page.getByTestId('artifact-runtime').first()).toHaveText('artifact-runtime/1');
  await expect(page.getByTestId('artifact-version-list').locator('li')).toHaveCount(2);
});

test('iframe uses sandbox="allow-scripts" only and never srcdoc', async ({ page }) => {
  await page.goto('/?fake=1');
  await openLibrary(page);
  const iframe = page.getByTestId('artifact-iframe');
  await expect(iframe).toHaveAttribute('sandbox', 'allow-scripts');
  expect(await iframe.getAttribute('srcdoc')).toBeNull();
  const src = await iframe.getAttribute('src');
  expect(src).toContain('artifact://abc123/');
  expect(await iframe.getAttribute('allow')).toBeNull();
});

test('main frame rejects navigation to artifact://', async ({ page }) => {
  await page.goto('/?fake=1');
  const before = page.url();
  await page.evaluate(() => {
    const link = document.createElement('a');
    link.href = 'artifact://evil/';
    link.dataset.testid = 'evil-artifact-link';
    link.textContent = 'open artifact in main frame';
    document.body.appendChild(link);
  });
  await page.locator('[data-testid="evil-artifact-link"]').click();
  expect(page.url()).toBe(before);
});

test('bridge accepts the whitelisted v1 messages', async ({ page }) => {
  await page.goto('/?fake=1');
  await openLibrary(page);

  await post(page, msg('resize', { width: 640, height: 480 }));
  await expect(page.getByTestId('artifact-size')).toHaveText('640×480');
  await expect.poll(() => lastDecision(page)).toEqual({ type: 'resize', accepted: true });

  await post(page, msg('reportError', { message: 'boom' }));
  await expect(page.getByTestId('artifact-error')).toHaveText('boom');

  await post(page, msg('saveState', { state: { step: 2 } }));
  await expect(page.getByTestId('artifact-saved')).toBeVisible();

  await post(page, msg('getData', { ref: 'artifact://data/points' }));
  await expect.poll(() => lastDecision(page)).toEqual({ type: 'getData', accepted: true });

  await post(page, msg('suggestPrompt', { prompt: 'jelaskan' }));
  await expect.poll(() => lastDecision(page)).toEqual({ type: 'suggestPrompt', accepted: true });

  await post(page, msg('loadState', {}));
  await expect.poll(() => lastDecision(page)).toEqual({ type: 'loadState', accepted: true });
});

test('bridge rejects wrong origin, wrong source, unknown types and oversized state', async ({ page }) => {
  await page.goto('/?fake=1');
  await openLibrary(page);

  await post(page, msg('resize', { width: 10, height: 10 }), { origin: 'https://evil.example' });
  await expect.poll(() => lastDecision(page)).toMatchObject({ accepted: false, reason: 'origin' });

  await post(page, msg('resize', { width: 10, height: 10 }), { source: 'parent' });
  await expect.poll(() => lastDecision(page)).toMatchObject({ accepted: false, reason: 'source' });

  await post(page, msg('eval', { code: 'alert(1)' }));
  await expect.poll(() => lastDecision(page)).toMatchObject({ accepted: false, reason: 'type' });

  await post(page, msg('saveState', { state: 'x'.repeat(40 * 1024) }));
  await expect.poll(() => lastDecision(page)).toMatchObject({ type: 'saveState', accepted: false, reason: 'too_large' });

  await post(page, { ...msg('resize', { width: 10, height: 10 }), channel: 'other' });
  await expect.poll(() => lastDecision(page)).toMatchObject({ accepted: false, reason: 'channel' });

  await expect(page.getByTestId('artifact-size')).toHaveCount(0);
});

test('streaming code is visible before storage and cannot run yet', async ({ page }) => {
  await page.goto('/?fake=1&stream=1');
  await openLibrary(page);
  const pending = page.getByTestId('artifact-streaming');
  await expect(pending.getByTestId('artifact-code')).toContainText('streaming');
  const iframe = pending.getByTestId('artifact-iframe');
  expect(await iframe.getAttribute('src')).toBeNull();

  await pending.getByTestId('artifact-store').click();
  await expect.poll(async () => await iframe.getAttribute('src')).toContain('artifact://pending/');
  await expect(pending.getByTestId('artifact-code')).toHaveCount(0);
});
