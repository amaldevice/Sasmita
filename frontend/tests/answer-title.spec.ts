import { expect, test } from '@playwright/test';

/**
 * DESIGN.md: "Judul jawaban 30px/700 (mode Chat), 36–50px (mode Baca)".
 * The title is a real heading with ID/EN copy, not an eyebrow label.
 */
test('answer title uses the Chat and Baca type scale', async ({ page }) => {
  await page.goto('/?fake=1&step=1');
  await page.getByTestId('composer-input').fill('apa itu fotosintesis');
  await page.getByTestId('send-button').click();

  await expect(page.getByTestId('assistant-answer')).toBeVisible();
  const title = page.getByTestId('answer-title').first();
  await expect(title).toBeVisible();
  await expect(title).toHaveText('Jawaban');
  expect(await title.evaluate((el) => getComputedStyle(el).fontSize)).toBe('30px');
  expect(await title.evaluate((el) => getComputedStyle(el).fontWeight)).toBe('700');

  await page.getByTestId('view-baca').click();
  const bacaSize = parseFloat(await title.evaluate((el) => getComputedStyle(el).fontSize));
  expect(bacaSize).toBeGreaterThanOrEqual(36);
  expect(bacaSize).toBeLessThanOrEqual(50);

  await page.getByTestId('toggle-language').click();
  await expect(title).toHaveText('Answer');
});
