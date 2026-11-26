import { expect, test } from '@playwright/test';

/**
 * Seam 2: attaching a file is an explicit affordance, and the selected file
 * travels with the run request. The fake IPC records the attachment names so
 * the assertion observes the IPC boundary, not component internals.
 */
test('attach a file, show it as a chip, and send it with the run', async ({ page }) => {
  await page.goto('/?fake=1');
  await page.getByTestId('attach-button').click();
  await expect(page.getByTestId('attachment-chip')).toContainText('note.txt');

  await page.getByTestId('composer-input').fill('read it');
  await page.getByTestId('send-button').click();

  await expect(page.getByTestId('attachment-chip')).toHaveCount(0);
  await expect.poll(() =>
    page.evaluate(() => {
      const state = JSON.parse(localStorage.getItem('sasmita.fake.v1') ?? '{}') as {
        attachmentsByRun?: Record<string, string[]>;
      };
      const all = Object.values(state.attachmentsByRun ?? {}).flat();
      return all.includes('note.txt');
    }),
  ).toBe(true);
});
