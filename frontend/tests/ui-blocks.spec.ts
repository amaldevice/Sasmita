import { expect, test } from '@playwright/test';

/**
 * Seam 2: a recorded ui.block stream renders Tier 1 components with direct
 * labels, no colour-box legend, and no big-number + small-label rows.
 */
test('renders chart, table, mermaid, KPI and math with direct labels', async ({ page }) => {
  await page.goto('/?fake=1&ui=1');
  await page.getByTestId('composer-input').fill('Tampilkan visual');
  await page.getByTestId('send-button').click();

  const block = page.getByTestId('ui-block');
  await expect(block).toBeVisible();
  await expect(block).toContainText('Compare deterministic and stochastic rounding.');
  await expect(block.getByTestId('ui-chart')).toBeVisible();
  await expect(block.getByTestId('ui-chart-point')).toHaveCount(3);
  await expect(block.getByTestId('ui-chart')).toContainText('Error by threshold');
  await expect(block.getByTestId('ui-table')).toBeVisible();
  // Rows come from the bounded artifact data API, not inline spec values.
  await expect(block.getByTestId('ui-table').locator('tbody tr')).toHaveCount(5);
  await expect(block.getByTestId('ui-mermaid-node')).toHaveCount(3);
  await expect(block.getByTestId('ui-mermaid')).toContainText('Start');
  await expect(block.getByTestId('ui-kpi-metric')).toHaveCount(2);
  await expect(block.getByTestId('ui-math')).toContainText('E = mc^2');
  await expect(page.getByTestId('legend')).toHaveCount(0);
});

/** View-local controls update the visual immediately and persist debounced. */
test('sliders, tabs and toggles work without a model call', async ({ page }) => {
  await page.goto('/?fake=1&ui=1');
  await page.getByTestId('composer-input').fill('Ubah parameter');
  await page.getByTestId('send-button').click();

  // Wait for the terminal reload so the interaction targets the persisted block.
  await expect(page.getByTestId('assistant-answer')).toContainText('Lihat visual berikut.');
  const block = page.getByTestId('ui-block');
  await expect(block.getByTestId('ui-chart-point')).toHaveCount(3);

  await block.getByTestId('ui-slider').evaluate((el: HTMLInputElement) => {
    el.value = '3';
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await expect(block.getByTestId('ui-chart-point')).toHaveCount(1);
  await expect(block.getByTestId('ui-control-slider')).toContainText('3');

  await block.getByTestId('ui-tab-sample').click();
  await expect(block.getByTestId('ui-chart')).toContainText('(sample)');
  await block.getByTestId('ui-toggle').check();
  await expect(block.getByTestId('ui-local-note')).toBeVisible();

  await page.waitForTimeout(450);
  const recorded = await page.evaluate(() => {
    const stored = JSON.parse(localStorage.getItem('sasmita.fake.v1') ?? '{}');
    const runId = stored.runs?.[0]?.runId as string;
    const events = stored.snapshots?.[runId]?.events as { type: string; payload: { patch?: Record<string, unknown> } }[];
    return {
      states: events.filter((event) => event.type === 'ui.state'),
      deltas: events.filter((event) => event.type === 'model.text_delta'),
    };
  });
  expect(recorded.states.length).toBeGreaterThan(0);
  expect(recorded.states.at(-1)?.payload.patch).toEqual({ showErrors: true });
  expect(recorded.deltas).toHaveLength(1);
});

/** The stored spec (and its artifact reference) survives a reload. */
test('re-renders the block from stored spec after reload', async ({ page }) => {
  await page.goto('/?fake=1&ui=1');
  await page.getByTestId('composer-input').fill('Simpan visual');
  await page.getByTestId('send-button').click();
  await expect(page.getByTestId('ui-block')).toBeVisible();

  await page.reload();
  await expect(page.getByTestId('ui-block')).toBeVisible();
  await expect(page.getByTestId('ui-block')).toContainText('Rounding explorer');
  await expect(page.getByTestId('ui-block').getByTestId('ui-chart-point')).toHaveCount(3);
  await expect(page.getByTestId('ui-block').getByTestId('ui-table').locator('tbody tr')).toHaveCount(5);
});

/** Invalid specs fail closed to text; the agent's textual answer is preserved. */
test('invalid spec falls back to text while preserving the answer', async ({ page }) => {
  await page.goto('/?fake=1&ui=fallback');
  await page.getByTestId('composer-input').fill('Coba visual');
  await page.getByTestId('send-button').click();

  await expect(page.getByTestId('ui-block-fallback')).toContainText('unknown component type');
  await expect(page.getByTestId('ui-block')).toHaveCount(0);
  await expect(page.getByTestId('assistant-answer')).toContainText('jawaban ini tetap tersimpan');
});
