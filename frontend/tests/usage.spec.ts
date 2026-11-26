import { expect, test } from '@playwright/test';

const QUESTION = 'What is a context note?';
const ANSWER = 'A context note is information that changed since the previous note.';

async function answerQuestion(page: import('@playwright/test').Page): Promise<void> {
  await page.goto('/?fake=1');
  await page.getByTestId('composer-input').fill(QUESTION);
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('assistant-answer').locator('p')).toHaveText(ANSWER, { timeout: 15_000 });
}

/**
 * Seam 2: usage is hidden until hover. The popover is derived from the
 * recorded model.usage event (12 input, 5 output, 3 cache reads = 25 %).
 */
test('hovering an answer shows usage with an app-estimated cost', async ({ page }) => {
  await answerQuestion(page);

  await expect(page.getByTestId('usage-line')).toHaveCount(0);
  // A hit-rate warning never appears in the chat.
  await expect(page.getByTestId('hit-rate-drop')).toHaveCount(0);

  await page.getByTestId('assistant-answer').hover();
  const popover = page.getByTestId('usage-popover');
  await expect(popover).toBeVisible();
  await expect(popover).toContainText('12');
  await expect(popover).toContainText('5');
  await expect(popover).toContainText('25%');
  await expect(popover).toContainText(/perkiraan aplikasi/i);
});

/** Seam 2: the setting shows the monospace usage line under each answer. */
test('always-show-usage setting adds a monospace line that survives reload', async ({ page }) => {
  await answerQuestion(page);

  await page.getByTestId('nav-settings').click();
  await page.getByTestId('toggle-always-usage').click();
  await expect(page.getByTestId('toggle-always-usage')).toHaveAttribute('aria-pressed', 'true');

  await page.getByTestId('recent-conversation').first().click();
  const line = page.getByTestId('usage-line');
  await expect(line).toBeVisible();
  await expect(line).toContainText('12');
  await expect(line).toContainText('25%');
  expect(await line.evaluate((el) => getComputedStyle(el).fontFamily)).toMatch(/mono/i);

  await page.reload();
  await page.getByTestId('recent-conversation').first().click();
  await expect(page.getByTestId('usage-line')).toBeVisible();
});

/** Seam 2: Diagnostics aggregates recorded usage per model and per day. */
test('diagnostics aggregates per model and day and flags a drop only there', async ({ page }) => {
  await page.goto('/?fake=1');
  await page.getByTestId('nav-settings').click();
  await page.getByTestId('open-diagnostics').click();

  await expect(page.getByTestId('diagnostics')).toBeVisible();
  await expect(page.getByTestId('model-row')).toHaveCount(2);
  await expect(page.getByTestId('day-row')).toHaveCount(3);
  await expect(page.getByTestId('diagnostics')).toContainText('gpt-test');
  await expect(page.getByTestId('diagnostics')).toContainText('claude-test');
  await expect(page.getByTestId('hit-rate-drop')).toBeVisible();

  await page.getByTestId('recent-conversation').first().click();
  await expect(page.getByTestId('diagnostics')).toHaveCount(0);
  await expect(page.getByTestId('hit-rate-drop')).toHaveCount(0);
});
