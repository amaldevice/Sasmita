import { expect, test } from '@playwright/test';

/**
 * Seam 2: the settings panel lists profiles and their models. Values the
 * provider did not report are rendered as "unknown", never guessed, and the
 * API key is never shown after it has been submitted.
 */
test('profiles, unknown capabilities and plain-language connection results', async ({ page }) => {
  await page.goto('/?fake=1');

  // #20 owns the shell: connections live inside its Settings view.
  await page.getByTestId('nav-settings').click();
  await page.getByTestId('open-connections').click();
  await expect(page.getByTestId('connections-panel')).toBeVisible();

  const profiles = page.getByTestId('profile-row');
  await expect(profiles).toHaveCount(3);

  await page.getByTestId('select-profile-profile-ok').click();
  await page.getByTestId('load-models').click();
  const models = page.getByTestId('model-row');
  await expect(models).toHaveCount(2);
  await expect(models.first()).toContainText('unknown');
  await expect(models.nth(1)).toContainText('200000');

  await page.getByTestId('test-connection').click();
  await expect(page.getByTestId('connection-result')).toContainText('Connected');

  await page.getByTestId('select-profile-profile-bad').click();
  await page.getByTestId('test-connection').click();
  await expect(page.getByTestId('connection-result')).toContainText('API key was rejected');

  await page.getByTestId('select-profile-profile-down').click();
  await page.getByTestId('test-connection').click();
  await expect(page.getByTestId('connection-result')).toContainText('Could not reach');

  // Adding a profile sends the key to Go only; it is never echoed back.
  await page.getByTestId('profile-name').fill('Local model');
  await page.getByTestId('profile-base-url').fill('http://localhost:11434');
  await page.getByTestId('profile-api-key').fill('secret-only-once');
  await page.getByTestId('save-profile').click();
  await expect(page.getByTestId('profile-row')).toHaveCount(4);
  await expect(page.locator('body')).not.toContainText('secret-only-once');
});
