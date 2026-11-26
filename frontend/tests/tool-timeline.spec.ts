import { expect, test } from '@playwright/test';

/**
 * Seam 2: the one-line activity summary expands into a tool timeline. The
 * recorded stream contains both proposals and executions, and an artifact with
 * a stable reference shown in chat.
 */
test('activity expands into a proposed/executed tool timeline', async ({ page }) => {
  await page.goto('/?fake=1&tools=1&step=700');
  await page.getByTestId('composer-input').fill('Hitung dan baca');
  await page.getByTestId('send-button').click();

  await expect(page.getByTestId('activity-line')).toContainText('Bekerja');
  await expect(page.getByTestId('activity-line')).toContainText('dtk');
  await expect(page.getByTestId('activity-spinner')).toBeVisible();
  await expect(page.getByTestId('timeline-toggle')).toBeVisible();

  await page.getByTestId('timeline-toggle').click();
  await expect(page.getByTestId('tool-timeline')).toBeVisible();

  const proposed = page.getByTestId('tool-step').filter({ hasText: 'usulan' });
  const executed = page.getByTestId('tool-step').filter({ hasText: 'dijalankan' });
  await expect(proposed.first()).toContainText('calculator');
  await expect(executed.first()).toContainText('calculator');
  await expect(proposed.nth(1)).toContainText('read_project_file');

  await expect(page.getByTestId('tool-artifact-ref')).toHaveText('artifact://abc123');
  await expect(page.getByTestId('tool-artifact-ref')).toBeVisible();
});
