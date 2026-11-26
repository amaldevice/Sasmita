import { expect, test } from '@playwright/test';

/**
 * Seam 2: the composer chips list every built-in Mode, but only Modes whose
 * tools exist in this phase can be picked. Model and Mode choices live next to
 * the message box, not in a separate settings screen.
 */
test('composer chips list Modes, mark future ones, and switch model', async ({ page }) => {
  await page.goto('/?fake=1');

  await expect(page.getByTestId('mode-chip')).toBeVisible();
  await expect(page.getByTestId('model-chip')).toBeVisible();

  await page.getByTestId('mode-chip').click();
  const menu = page.getByTestId('mode-menu');
  await expect(menu).toBeVisible();
  await expect(menu.getByTestId('mode-option')).toHaveCount(5);
  await expect(page.getByTestId('mode-option-analis-data')).toBeDisabled();
  await expect(page.getByTestId('mode-option-analis-data')).toContainText('Segera hadir');
  await expect(page.getByTestId('mode-option-coding')).toBeDisabled();

  // A short conversation must not nag: the switch applies immediately.
  await page.getByTestId('mode-option-riset').click();
  await expect(page.getByTestId('mode-chip')).toContainText('Riset');
  await expect(page.getByTestId('reread-warning')).toHaveCount(0);

  await page.getByTestId('model-chip').click();
  await expect(page.getByTestId('model-menu').getByTestId('model-option')).toHaveCount(2);
  await page.getByTestId('model-option-model-b').click();
  await expect(page.getByTestId('model-chip')).toContainText('model-b');
});

/**
 * Seam 2: past ~4,000 tokens, changing Mode or model asks for confirmation
 * and states the estimated re-read cost. Cancelling keeps the old choice.
 */
test('long history warns with an estimated re-read before switching', async ({ page }) => {
  await page.goto('/?fake=1');
  await page.getByTestId('composer-input').fill('a'.repeat(20000));
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('assistant-answer')).toHaveCount(1);

  await page.getByTestId('mode-chip').click();
  await page.getByTestId('mode-option-penulisan').click();
  const warning = page.getByTestId('reread-warning');
  await expect(warning).toBeVisible();
  await expect(warning).toContainText('Dibaca ulang penuh');
  await expect(warning).toContainText(/\d/);

  await page.getByTestId('reread-cancel').click();
  await expect(page.getByTestId('mode-chip')).toContainText('Chat');

  await page.getByTestId('mode-chip').click();
  await page.getByTestId('mode-option-penulisan').click();
  await page.getByTestId('reread-confirm').click();
  await expect(page.getByTestId('mode-chip')).toContainText('Penulisan');
  await expect(page.getByTestId('reread-warning')).toHaveCount(0);
});
