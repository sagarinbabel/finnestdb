import { expect, test } from '@playwright/test';

const mixedFinnishText = 'Viisutubettaja lauloi nopeasti. Menin pankkiin eilen.';

test('results page shows parser metadata, sorting, and POS filters', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Custom Parser' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#results-parser')).toHaveText('Custom parser');
  await expect(page.locator('#coverage-value')).toContainText('%');
  await expect(page.locator('#results-help')).toContainText('Coverage = dictionary-backed tokens.');
  await expect(page.locator('th.col-count')).toContainText('Tokens');

  const rowNumbers = page.locator('tbody#word-table-body td.col-row');
  await expect(rowNumbers.first()).toHaveText('1');

  const definitionHeader = page.getByRole('button', { name: /Definition/ });
  await definitionHeader.click();

  const firstDefinitionCell = page.locator('tbody#word-table-body td.col-def').first();
  await expect(firstDefinitionCell).not.toContainText('Missing');

  await definitionHeader.click();
  await expect(firstDefinitionCell).toContainText('Missing');

  const initialRowCount = await page.locator('tbody#word-table-body tr').count();
  await page.getByRole('button', { name: 'Nouns' }).click();
  await expect(page.locator('.pos-filter-chip.active')).toHaveText('Nouns');
  const nounRowCount = await page.locator('tbody#word-table-body tr').count();
  expect(nounRowCount).toBeGreaterThan(0);
  expect(nounRowCount).toBeLessThan(initialRowCount);

  const posPills = page.locator('tbody#word-table-body td.col-pos .pos-pill');
  const pillCount = await posPills.count();
  for (let i = 0; i < pillCount; i++) {
    await expect(posPills.nth(i)).toContainText(/Noun|Proper noun/);
  }
});

test('language mismatch warning blocks parse until switching languages', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Language').selectOption('ET');
  await page.getByLabel('Text').fill('Menin pankkiin tänään ja söin hyvää leipää.');

  await expect(page.locator('#lang-warning')).toContainText('looks like Finnish');
  await expect(page.locator('#lang-switch-btn')).toBeVisible();
  await expect(page.locator('#parse-btn-custom')).toBeDisabled();
  await expect(page.locator('#parse-btn-basic')).toBeDisabled();

  await page.getByRole('button', { name: 'Switch to Finnish' }).click();

  await expect(page.getByLabel('Language')).toHaveValue('FI');
  await expect(page.locator('#lang-warning')).toBeHidden();
  await expect(page.locator('#parse-btn-custom')).toBeEnabled();
  await expect(page.locator('#parse-btn-basic')).toBeEnabled();
});

test('file upload loads text and can be parsed', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Language').selectOption('ET');
  await page.locator('#parse-file').setInputFiles({
    name: 'sample.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('Rõõmus kass jooksis kiiresti koju.'),
  });

  await expect(page.getByLabel('Text')).toHaveValue('Rõõmus kass jooksis kiiresti koju.');
  await expect(page.locator('#char-count')).toContainText('34 / 300,000');

  await page.getByRole('button', { name: 'Custom Parser' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#results-title')).toContainText('(Estonian)');
});

test('nav shell stays focused on the workbench', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');

  await expect(page.getByRole('link', { name: 'FinEstDB' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Dashboard' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Catalog' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Review' })).toHaveCount(0);

  await page.getByLabel('Text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Custom Parser' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  await page.getByLabel('Menu').click();
  await expect(page.locator('#nav-mobile-overlay')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Workbench' })).toBeVisible();
  await page.locator('#nav-mobile-import').click();

  await expect(page.locator('#parse-page')).toHaveClass(/active/);
  await expect(page.locator('#nav-mobile-overlay')).toBeHidden();
});
