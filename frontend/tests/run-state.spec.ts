import { expect, test } from '@playwright/test';

const QUESTION = 'What is a context note?';
const ANSWER = 'A context note is information that changed since the previous note.';

/** Seam 2: CancelRun stops the replay and the run settles as cancelled. */
test('cancel stops a streaming run', async ({ page }) => {
  await page.goto('/?fake=1&step=800');
  await page.getByTestId('composer-input').fill(QUESTION);
  await page.getByTestId('send-button').click();

  await expect(page.getByTestId('streaming-answer')).toBeVisible();
  await page.getByTestId('cancel-run').click();

  await expect(page.getByTestId('run-error')).toContainText('run cancelled');
  await expect(page.getByTestId('interrupted-run')).toHaveCount(0);
  await expect(page.getByTestId('assistant-answer')).toHaveCount(0);
});

/** Seam 2: a run left active by a reload is interrupted and offers Continue. */
test('interrupted run offers continue after restart', async ({ page }) => {
  await page.goto('/?fake=1&step=800');
  await page.getByTestId('composer-input').fill(QUESTION);
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('streaming-answer')).toBeVisible();

  // Reload mid-stream: the fake IPC persists the active run and the next
  // startup recovers it, mirroring appsvc.RecoverStaleRuns.
  await page.reload();

  await expect(page.getByTestId('interrupted-run')).toBeVisible();
  await expect(page.getByTestId('continue-run')).toBeVisible();
  await page.getByTestId('continue-run').click();

  await expect(page.getByTestId('assistant-answer').locator('p')).toHaveText(ANSWER, { timeout: 15_000 });
  await expect(page.getByTestId('interrupted-run')).toHaveCount(0);
  await expect(page.getByTestId('user-message').locator('p')).toHaveText(QUESTION);
});
