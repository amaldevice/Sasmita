import { expect, test } from '@playwright/test';

/**
 * Seam 2: manual compaction is reachable in Settings and reports the stored
 * structured summary. The fake IPC records the same typed command the shell
 * binding exposes.
 */
test('manual compaction control records a compaction summary', async ({ page }) => {
  await page.goto('/?fake=1');
  await page.getByTestId('nav-settings').click();
  await expect(page.getByTestId('settings-view')).toBeVisible();
  await page.getByTestId('compact-button').click();
  await expect(page.getByTestId('compaction-status')).toContainText('ringkasan percakapan');
});
