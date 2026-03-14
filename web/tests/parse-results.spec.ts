import { expect, test } from '@playwright/test';

test('results page shows parser metadata and sortable missing definitions', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Text').fill(
    'Viisutubettaja lauloi. Menin pankkiin. Menin kotiin.'
  );

  await page.getByRole('button', { name: 'Custom Parser' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#results-parser')).toHaveText('Custom parser');
  await expect(page.locator('#results-score')).toContainText('Coverage score');
  await expect(page.locator('#results-help')).toContainText(
    'Coverage score = how much of this text produced usable dictionary-backed output.'
  );
  await expect(page.locator('th.col-count')).toContainText('Tokens');

  const rowNumbers = page.locator('tbody#word-table-body td.col-row');
  await expect(rowNumbers.first()).toHaveText('1');

  const definitionHeader = page.getByRole('button', { name: /Definition/ });
  await definitionHeader.click();

  const firstDefinitionCell = page.locator('tbody#word-table-body td.col-def').first();
  await expect(firstDefinitionCell).not.toContainText('Missing');

  await definitionHeader.click();
  await expect(firstDefinitionCell).toContainText('Missing');
});
