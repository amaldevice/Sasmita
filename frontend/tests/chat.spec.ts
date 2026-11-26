import { expect, test } from '@playwright/test';

const QUESTION = 'What is a context note?';
const ANSWER = 'A context note is information that changed since the previous note.';

/**
 * Seam 2: the frontend replays a recorded RunEvent JSON stream through fake IPC
 * and must render the coalesced answer, then recover the same history on reload.
 */
test('streamed answer renders and survives a restart', async ({ page }) => {
  await page.goto('/?fake=1&slow=1');
  await expect(page.getByTestId('thread')).toBeVisible();

  await page.getByTestId('composer-input').fill(QUESTION);
  await page.getByTestId('send-button').click();

  const userText = page.getByTestId('user-message').last().locator('p');
  await expect(userText).toHaveText(QUESTION);

  // The answer arrives progressively: an early render is a strict prefix of
  // the final answer, proving deltas are coalesced into the same message node.
  const streamingText = page.getByTestId('streaming-answer').locator('p');
  await expect(streamingText).toBeVisible();
  await expect.poll(async () => (await streamingText.textContent())?.length ?? 0).toBeGreaterThan(0);
  const partial = (await streamingText.textContent()) ?? '';

  await expect(page.getByTestId('assistant-answer').locator('p')).toHaveText(ANSWER, { timeout: 15_000 });
  expect(ANSWER.startsWith(partial)).toBe(true);
  await expect(page.getByTestId('usage')).toContainText('12 ↑');

  // Reload with the same fake IPC state = conversation reopened after restart.
  await page.reload();
  await expect(page.getByTestId('user-message').locator('p')).toHaveText(QUESTION);
  await expect(page.getByTestId('assistant-answer').locator('p')).toHaveText(ANSWER);
});
