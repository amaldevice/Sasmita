import { expect, test } from '@playwright/test';

/**
 * Seam 2: onboarding runs entirely on fake IPC. `onboarding=1` forces the
 * first-run screen; `detect=` controls which local endpoints the fake probe
 * answers, so no real port is ever touched in the browser tests.
 */
test('welcome explains Sasmita in one screen, ID and EN', async ({ page }) => {
  await page.goto('/?fake=1&onboarding=1&detect=ollama');

  await expect(page.getByTestId('onboarding')).toBeVisible();
  await expect(page.getByTestId('welcome-title')).toContainText('Sasmita');
  await expect(page.getByTestId('welcome-body')).toBeVisible();

  const detected = page.getByTestId('detected-endpoint');
  await expect(detected).toHaveCount(1);
  await expect(detected).toContainText('Ollama');
  await expect(detected).toContainText('11434');
  // Detection is not a connection claim.
  await expect(detected).toContainText('belum diuji');
  await expect(page.getByTestId('use-api')).toBeVisible();

  await page.getByTestId('onboarding-language').click();
  await expect(page.getByTestId('welcome-body')).toContainText('visual');
  await expect(detected).toContainText('not tested');
  await expect(page.getByTestId('use-api')).toHaveText('Use an API');
});

test('no running endpoint is never presented as a local connection', async ({ page }) => {
  await page.goto('/?fake=1&onboarding=1&detect=none');

  await expect(page.getByTestId('detected-endpoint')).toHaveCount(0);
  await expect(page.getByTestId('no-local')).toBeVisible();
  await expect(page.getByTestId('use-api')).toBeVisible();
});

test('use an API stores the key behind the provider port and tests the connection', async ({ page }) => {
  await page.goto('/?fake=1&onboarding=1&detect=none');
  await page.getByTestId('use-api').click();

  await page.getByTestId('onboarding-kind').selectOption('openai-compatible');
  await page.getByTestId('onboarding-name').fill('My API');
  await page.getByTestId('onboarding-base-url').fill('https://api.example.test/v1');
  await page.getByTestId('onboarding-api-key').fill('sk-onboarding-secret');
  await page.getByTestId('onboarding-save-test').click();

  await expect(page.getByTestId('onboarding-result')).toContainText('Connected');
  await page.getByTestId('onboarding-continue').click();
  await expect(page.getByTestId('onboarding')).toHaveCount(0);
  await expect(page.getByTestId('thread')).toBeVisible();
  // The key went to the keychain port; it is never echoed to the UI.
  await expect(page.locator('body')).not.toContainText('sk-onboarding-secret');
});

test('a detected endpoint is offered and prefilled for the connection test', async ({ page }) => {
  await page.goto('/?fake=1&onboarding=1&detect=all');
  await expect(page.getByTestId('detected-endpoint')).toHaveCount(2);

  await page.getByTestId('detected-endpoint').first().click();
  await expect(page.getByTestId('onboarding-kind')).toHaveValue('openai-compatible');
  await expect(page.getByTestId('onboarding-base-url')).toHaveValue('http://127.0.0.1:11434/v1');
  await page.getByTestId('onboarding-save-test').click();

  await expect(page.getByTestId('onboarding-result')).toContainText('Connected');
  await page.getByTestId('onboarding-continue').click();
  await expect(page.getByTestId('onboarding')).toHaveCount(0);
});

test('empty state has a heading, one sentence and 2x2 cards wired to the run path', async ({ page }) => {
  await page.goto('/?fake=1');

  await expect(page.getByTestId('empty-state')).toBeVisible();
  await expect(page.getByTestId('empty-title')).toHaveText('Mulai dari pertanyaan pertama');
  await expect(page.getByTestId('empty-body')).toContainText('Pilih salah satu contoh');

  const cards = page.getByTestId('suggestion-card');
  await expect(cards).toHaveCount(4);
  const columns = await page.getByTestId('suggestions').evaluate(
    (element) => getComputedStyle(element).gridTemplateColumns.split(' ').length,
  );
  expect(columns).toBe(2);

  await page.getByTestId('toggle-language').click();
  await expect(page.getByTestId('empty-title')).toHaveText('Start with a first question');

  const prompt = 'Jelaskan alur training neural network dengan diagram';
  await cards.first().click();
  await expect(page.getByTestId('user-message').locator('p')).toHaveText(prompt);
  await expect(page.getByTestId('assistant-answer').locator('p')).toBeVisible({ timeout: 15_000 });
});
