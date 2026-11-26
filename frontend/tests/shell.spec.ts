import { expect, test } from '@playwright/test';

const ANSWER = 'A context note is information that changed since the previous note.';

/** Seam 2: the shell is driven entirely through fake IPC, no Go process. */
test.beforeEach(async ({ page }) => {
  await page.goto('/?fake=1');
  await expect(page.getByTestId('thread')).toBeVisible();
});

test('sidebar collapses to a 64px rail by button and by Cmd+\\', async ({ page }) => {
  const sidebar = page.getByTestId('sidebar');
  await expect(sidebar).toHaveAttribute('data-state', 'expanded');
  expect((await sidebar.boundingBox())?.width).toBeGreaterThan(200);

  await page.getByTestId('collapse-button').click();
  await expect(sidebar).toHaveAttribute('data-state', 'rail');
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBe(64);
  // Labels are hidden in the rail but remain available as accessible names.
  await expect(page.getByTestId('nav-search')).toBeVisible();
  await expect(page.getByTestId('nav-search')).toHaveAttribute('aria-label', 'Cari');

  await page.keyboard.press('Meta+Backslash');
  await expect(sidebar).toHaveAttribute('data-state', 'expanded');
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBeGreaterThan(200);

  await page.keyboard.press('Meta+Backslash');
  await expect(sidebar).toHaveAttribute('data-state', 'rail');
});

test('theme switches dark/light tokens and persists across reload', async ({ page }) => {
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  const darkBackground = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);

  await page.getByTestId('toggle-theme').click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  const lightBackground = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(lightBackground).not.toBe(darkBackground);
  expect(lightBackground).toBe('rgb(237, 236, 232)'); // DESIGN.md --ground, light

  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});

test('language switches every shell string and persists across reload', async ({ page }) => {
  await expect(page.getByTestId('nav-search')).toHaveText('Cari');
  await expect(page.getByTestId('composer-input')).toHaveAttribute('placeholder', 'Tanya sesuatu…');
  await expect(page.getByTestId('rename-button')).toHaveText('Ubah nama');

  await page.getByTestId('toggle-language').click();
  await expect(page.getByTestId('nav-search')).toHaveText('Search');
  await expect(page.getByTestId('composer-input')).toHaveAttribute('placeholder', 'Ask anything…');
  await expect(page.getByTestId('rename-button')).toHaveText('Rename');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');

  await page.reload();
  await expect(page.getByTestId('nav-search')).toHaveText('Search');
});

test('settings view offers explicit theme and language choices', async ({ page }) => {
  await page.getByTestId('nav-settings').click();
  await expect(page.getByTestId('settings-view')).toBeVisible();
  await page.getByTestId('theme-light').click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.getByTestId('lang-en').click();
  await expect(page.getByTestId('settings-view').getByRole('heading')).toHaveText('Settings');
});

test('search finds conversations by title and by message text', async ({ page }) => {
  await page.getByTestId('composer-input').fill('why is a context note useful');
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('assistant-answer').locator('p')).toHaveText(ANSWER, { timeout: 15_000 });

  await page.getByTestId('nav-search').click();
  await page.getByTestId('search-input').fill('context');
  await page.getByTestId('search-submit').click();
  await expect(page.getByTestId('search-result').first()).toContainText('why is a context note useful');

  // Titles are indexed too: a rename must immediately change search hits.
  await page.getByTestId('search-result').first().click();
  await expect(page.getByTestId('thread')).toBeVisible();
  await page.getByTestId('rename-button').click();
  await page.getByTestId('rename-input').fill('Context title check');
  await page.getByTestId('rename-save').click();
  await page.getByTestId('nav-search').click();
  await page.getByTestId('search-input').fill('title check');
  await page.getByTestId('search-submit').click();
  await expect(page.getByTestId('search-result').first()).toContainText('Context title check');
});

test('rename and move a conversation; no folder without a project', async ({ page }) => {
  await expect(page.getByTestId('open-folder')).toBeVisible();

  await page.getByTestId('rename-button').click();
  await page.getByTestId('rename-input').fill('Renamed chat');
  await page.getByTestId('rename-save').click();
  await expect(page.getByTestId('recent-conversation').first()).toHaveText('Renamed chat');
  await expect(page.getByRole('heading', { name: 'Renamed chat' })).toBeVisible();

  await page.getByTestId('move-button').click();
  await page.getByTestId('move-select').selectOption('');
  await page.getByTestId('move-save').click();
  await expect(page.getByTestId('open-folder')).toHaveCount(0);
});

test('reduced motion removes the rail transition', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const duration = await page.getByTestId('sidebar').evaluate((el) => getComputedStyle(el).transitionDuration);
  expect(duration).toBe('0s');
});
