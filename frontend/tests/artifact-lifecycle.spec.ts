import { expect, test, type Page } from '@playwright/test';

/**
 * Seam 2: the artifact lifecycle renders the RenderFlow allocation from the
 * prototype. The harness drives the same actions a browser would (scroll,
 * reopen, watchdog callback) and asserts rendered surfaces, never internals.
 */
async function openLifecycle(page: Page): Promise<void> {
  await page.goto('/?fake=1');
  await page.getByTestId('nav-artifacts').click();
  await expect(page.getByTestId('artifact-lifecycle')).toBeVisible();
}

test('allocation honours expanded, WebGL cap and two live slots', async ({ page }) => {
  await openLifecycle(page);
  const root = page.getByTestId('artifact-lifecycle');
  const live = root.getByTestId('lifecycle-live');
  await expect(live).toHaveCount(2);
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="webgl-a"]')).toBeVisible();
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="plain-d"]')).toBeVisible();
  // webgl-c is newer but cannot take the second WebGL slot, so d wins instead.
  await expect(root.locator('[data-testid="artifact-card"][data-block="webgl-c"][data-view="slots_full"]')).toBeVisible();
  await expect(root.locator('[data-testid="artifact-card"][data-block="plain-b"][data-view="slots_full"]')).toBeVisible();
  await expect(live.locator('iframe')).toHaveCount(2);
});

test('off-screen artifacts save state, lose the iframe and show a Run card', async ({ page }) => {
  await openLifecycle(page);
  const root = page.getByTestId('artifact-lifecycle');
  await root.locator('[data-block="webgl-a"]').getByTestId('lifecycle-offscreen').click();

  const parked = root.locator('[data-testid="artifact-card"][data-block="webgl-a"][data-view="parked"]');
  await expect(parked).toBeVisible();
  await expect(parked).toContainText('3D orbit');
  await expect(parked).toContainText('v2');
  await expect(parked).toContainText('Simulasi orbit 3D');
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="webgl-a"]')).toHaveCount(0);
  const saved = await page.evaluate(() => localStorage.getItem('artifact.state.webgl-a'));
  expect(saved).not.toBeNull();

  await parked.getByTestId('artifact-run').click();
  const relaunched = root.locator('[data-testid="lifecycle-live"][data-block="webgl-a"]');
  await expect(relaunched).toBeVisible();
  await expect(relaunched).toContainText('state restored');
});

test('reopening a conversation reallocates only visible artifacts', async ({ page }) => {
  await openLifecycle(page);
  const root = page.getByTestId('artifact-lifecycle');
  await root.getByTestId('lifecycle-reopen').click();
  await expect(root.getByTestId('lifecycle-live')).toHaveCount(2);
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="webgl-c"]')).toBeVisible();
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="plain-d"]')).toBeVisible();
  await expect(root.locator('[data-testid="artifact-card"][data-block="plain-b"][data-view="parked"]')).toBeVisible();
});

test('watchdog freezes the offender and offers retry, separate window and code', async ({ page }) => {
  await page.addInitScript(() => {
    (window as unknown as Record<string, unknown>).__markFrozeCalls = [];
    (window as unknown as Record<string, unknown>).__sasmitaMarkFroze = (id: string) => {
      ((window as unknown as Record<string, string[]>).__markFrozeCalls).push(id);
    };
    (window as unknown as Record<string, unknown>).__untrustedWindows = [];
    (window as unknown as Record<string, unknown>).__sasmitaOpenUntrusted = (uri: string, untrusted: boolean) => {
      ((window as unknown as Record<string, unknown[]>).__untrustedWindows).push({ uri, untrusted });
    };
  });
  await openLifecycle(page);
  const root = page.getByTestId('artifact-lifecycle');
  await page.evaluate(() => {
    const hook = (window as unknown as Record<string, (id: string) => void>).__sasmitaWatchdog;
    hook('webgl-a');
  });

  const frozen = root.locator('[data-testid="artifact-card"][data-block="webgl-a"][data-view="froze_view"]');
  await expect(frozen).toBeVisible();
  await expect(frozen).toContainText('Membekukan tampilan');
  await expect(frozen.getByTestId('artifact-retry')).toBeVisible();
  await expect(frozen.getByTestId('artifact-open-window')).toBeVisible();
  await expect(frozen.getByTestId('artifact-view-code')).toBeVisible();
  expect(await page.evaluate(() => (window as unknown as Record<string, string[]>).__markFrozeCalls)).toContain('webgl-a');
  // The freed WebGL slot is reallocated, but the offender never comes back.
  await expect(root.locator('[data-testid="lifecycle-live"]')).toHaveCount(2);
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="webgl-a"]')).toHaveCount(0);

  await frozen.getByTestId('artifact-open-window').click();
  await expect(root.locator('[data-testid="artifact-card"][data-block="webgl-a"][data-view="in_window"]')).toBeVisible();
  expect(await page.evaluate(() => (window as unknown as Record<string, unknown[]>).__untrustedWindows)).toContainEqual({
    uri: 'artifact://webgl-a/', untrusted: true,
  });
});

test('frozen artifact retries only on an explicit user Run', async ({ page }) => {
  await openLifecycle(page);
  const root = page.getByTestId('artifact-lifecycle');
  await page.evaluate(() => {
    (window as unknown as Record<string, (id: string) => void>).__sasmitaWatchdog('webgl-a');
  });
  const frozen = root.locator('[data-testid="artifact-card"][data-block="webgl-a"]');
  await frozen.getByTestId('artifact-retry').click();
  await expect(root.locator('[data-testid="lifecycle-live"][data-block="webgl-a"]')).toBeVisible();
});

test('a block streaming when the app closed is interrupted with a continue action', async ({ page }) => {
  await openLifecycle(page);
  const root = page.getByTestId('artifact-lifecycle');
  await root.getByTestId('lifecycle-reopen').click();
  const interrupted = root.locator('[data-testid="artifact-card"][data-block="stream-draft"][data-view="interrupted"]');
  await expect(interrupted).toBeVisible();
  await expect(interrupted).toContainText('Minta model melanjutkan');
  await expect(interrupted.getByTestId('artifact-ask-continue')).toBeVisible();
});
