import { expect, test } from '@playwright/test';

/**
 * Seam 2: the approval card is an inline, plain-language surface that pulses
 * until the user answers. The fake IPC replays a recorded RunEvent stream and
 * pauses on the approval event, so the decision actually drives the run.
 */
test('approval card shows action, resource and impact, then allows once', async ({ page }) => {
  await page.goto('/?fake=1&approval=1&step=120');
  await page.getByTestId('composer-input').fill('Timpa chart');
  await page.getByTestId('send-button').click();

  const card = page.getByTestId('approval-card');
  await expect(card).toBeVisible();
  await expect(card).toContainText('Menimpa artifact');
  await expect(card).toContainText('chart');
  await expect(card).toContainText('Isi lama akan diganti');
  await expect(card).toHaveAttribute('data-pulsing', 'true');

  await page.getByTestId('approve-once').click();
  await expect(card).toHaveCount(0);
  await expect(page.getByTestId('assistant-answer').locator('p')).toHaveText('Selesai.', { timeout: 15_000 });
});

test('deny records a denied tool step and never executes it', async ({ page }) => {
  await page.goto('/?fake=1&approval=1&step=120');
  await page.getByTestId('composer-input').fill('Timpa chart');
  await page.getByTestId('send-button').click();

  await expect(page.getByTestId('approval-card')).toBeVisible();
  await page.getByTestId('deny-approval').click();
  await expect(page.getByTestId('approval-card')).toHaveCount(0);
  await expect(page.getByTestId('timeline-toggle')).toBeVisible();
  await page.getByTestId('timeline-toggle').click();
  await expect(page.locator('[data-testid="tool-step"][data-phase="denied"]')).toBeVisible();
});

test('sidebar badge marks a conversation waiting elsewhere', async ({ page }) => {
  await page.addInitScript(() => {
    const now = new Date().toISOString();
    localStorage.setItem('sasmita.fake.v1', JSON.stringify({
      conversations: [{ id: 'conv-a', projectId: 'p1', title: 'Other chat', activeHarnessId: 'chat', createdAt: now, updatedAt: now }],
      details: { 'conv-a': { conversation: { id: 'conv-a', projectId: 'p1', title: 'Other chat', activeHarnessId: 'chat', createdAt: now, updatedAt: now }, messages: [] } },
      snapshots: {},
      runs: [{ runId: 'r-wait', conversationId: 'conv-a', status: 'awaiting_approval', answer: '' }],
      projects: [{ id: 'p1', name: 'Work', rootUri: '', createdAt: now, updatedAt: now }],
    }));
  });
  await page.goto('/?fake=1');
  const badge = page.getByTestId('recent-conversation').getByTestId('sidebar-badge');
  await expect(badge).toBeVisible();
  await expect(badge).toHaveAttribute('data-kind', 'waiting');
});

test('notification hook fires only when the app is not frontmost', async ({ page }) => {
  await page.addInitScript(() => {
    (window as unknown as Record<string, unknown>).__notifications = [];
    (window as unknown as Record<string, unknown>).__sasmitaNotify = (title: string) => {
      ((window as unknown as Record<string, string[]>).__notifications).push(title);
    };
    document.hasFocus = () => false;
  });
  await page.goto('/?fake=1&approval=1&step=120');
  await page.getByTestId('composer-input').fill('Timpa chart');
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('approval-card')).toBeVisible();
  await expect.poll(() => page.evaluate(() => (window as unknown as Record<string, string[]>).__notifications.length)).toBe(1);
});

test('notification hook stays quiet while the app is frontmost', async ({ page }) => {
  await page.addInitScript(() => {
    (window as unknown as Record<string, unknown>).__notifications = [];
    (window as unknown as Record<string, unknown>).__sasmitaNotify = (title: string) => {
      ((window as unknown as Record<string, string[]>).__notifications).push(title);
    };
    document.hasFocus = () => true;
  });
  await page.goto('/?fake=1&approval=1&step=120');
  await page.getByTestId('composer-input').fill('Timpa chart');
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('approval-card')).toBeVisible();
  await page.waitForTimeout(300);
  expect(await page.evaluate(() => (window as unknown as Record<string, string[]>).__notifications.length)).toBe(0);
});
