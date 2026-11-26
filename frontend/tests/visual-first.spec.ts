import { expect, test } from '@playwright/test';

/**
 * Seam 2: answers are visual-first. The recorded stream emits a visual tool
 * call, then its UI block, then the text. With slow replay the rendered DOM
 * must preserve that order: skeleton, visual, streaming text.
 */
test('renders skeleton, then visual, then streaming text', async ({ page }) => {
  await page.goto('/?fake=1&visual=1&step=180');
  await page.evaluate(() => {
    const order: string[] = [];
    const ids = ['visual-skeleton', 'ui-block', 'streaming-answer'];
    (window as unknown as Record<string, unknown>).__visualOrder = order;
    const observer = new MutationObserver(() => {
      for (const id of ids) {
        if (!order.includes(id) && document.querySelector(`[data-testid="${id}"]`)) order.push(id);
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
    (window as unknown as Record<string, unknown>).__visualObserver = observer;
  });

  await page.getByTestId('composer-input').fill('Jelaskan transformer');
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('assistant-answer')).toContainText('Berikut penjelasannya.');

  const order = await page.evaluate(() => (window as unknown as { __visualOrder: string[] }).__visualOrder);
  expect(order).toEqual(['visual-skeleton', 'ui-block', 'streaming-answer']);
  await expect(page.getByTestId('visual-skeleton')).toHaveCount(0);
});

/** The three answer formats have distinct, observable column layouts. */
test('formats switch between one column, two below, and side by side', async ({ page }) => {
  await page.goto('/?fake=1&visual=1');
  const column = page.getByTestId('reading-column');
  await expect(column).toHaveAttribute('data-format', 'one');
  await expect(page.getByTestId('format-one')).toHaveAttribute('aria-pressed', 'true');

  await page.getByTestId('composer-input').fill('Tampilkan visual');
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('ui-block')).toBeVisible();
  const visualGrid = page.getByTestId('visual-grid');
  expect(await visualGrid.evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(1);

  await page.getByTestId('format-two-below').click();
  await expect(column).toHaveAttribute('data-format', 'twoBelow');
  expect(await visualGrid.evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(2);

  await page.getByTestId('format-side-by-side').click();
  await expect(column).toHaveAttribute('data-format', 'sideBySide');
  const answerGrid = page.getByTestId('answer-group').first();
  expect(await answerGrid.evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(2);
});

/** Chat | Baca is a header switch; Baca uses the rail and a 1040px column. */
test('Chat and Baca views switch the rail and reading width', async ({ page }) => {
  await page.goto('/?fake=1');
  await expect(page.getByTestId('view-chat')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('sidebar')).toHaveAttribute('data-state', 'expanded');
  expect((await page.getByTestId('reading-column').boundingBox())?.width).toBeLessThanOrEqual(760);

  await page.getByTestId('view-baca').click();
  await expect(page.getByTestId('view-baca')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByTestId('sidebar')).toHaveAttribute('data-state', 'rail');
  expect((await page.getByTestId('sidebar').boundingBox())?.width).toBe(64);
  const column = page.getByTestId('reading-column');
  await expect(column).toHaveAttribute('data-view', 'baca');
  await expect(column).toHaveAttribute('data-format', 'twoBelow');
  expect((await column.boundingBox())?.width).toBe(1040);

  await page.getByTestId('view-chat').click();
  await expect(page.getByTestId('sidebar')).toHaveAttribute('data-state', 'expanded');
  await expect(column).toHaveAttribute('data-format', 'one');
});

/** The inline spot opens the side panel; full screen exits with Esc. */
test('artifact opens beside chat, goes full screen, and exits with Escape', async ({ page }) => {
  await page.goto('/?fake=1&tools=1');
  await page.getByTestId('toggle-language').click(); // assert the English copy too
  await page.getByTestId('composer-input').fill('Jelaskan dengan artifact');
  await page.getByTestId('send-button').click();

  const spot = page.getByTestId('artifact-inline-spot');
  await expect(spot).toContainText('Open in side panel');
  await page.getByTestId('open-beside-chat').click();

  const panel = page.getByTestId('artifact-panel');
  await expect(panel).toHaveAttribute('data-state', 'side');
  await expect(panel.getByTestId('artifact-iframe')).toBeVisible();

  await page.getByTestId('artifact-fullscreen').click();
  await expect(panel).toHaveAttribute('data-state', 'fullscreen');
  await page.keyboard.press('Escape');
  await expect(panel).toHaveAttribute('data-state', 'side');
  await page.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);
  await expect(spot).toBeVisible();
});
