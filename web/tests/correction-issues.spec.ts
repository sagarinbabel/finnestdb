import { expect, test, type Page, type Route } from '@playwright/test';

// Guards the Phase 1c admin correction-issues surface on the combined parser
// feedback page:
//   1. Issues render with scope, report/distinct-reporter counts, and a
//      threshold-candidate badge.
//   2. "Quarantine now" is gated: no class -> blocked; class + reason -> sends
//      the right PATCH (action=quarantine, alpha_class, reason).
//   3. After quarantine the issue re-renders as Quarantined with a Restore
//      action, and restoring sends action=restore.
//
// API calls are mocked so the test doesn't depend on the live Go server or DB.

const OPEN_ISSUE = {
  id: 7,
  lang: 'FI',
  parser: 'custom',
  norm_surface: 'koira',
  lemma: 'koira',
  pos: 'NOUN',
  status: 'open',
  report_count: 3,
  distinct_reporter_count: 3,
  reopened_count: 0,
  threshold_candidate: true,
};

const QUARANTINED_ISSUE = { ...OPEN_ISSUE, status: 'quarantined', threshold_candidate: false, quarantine_reason: 'parser identity wrong' };

async function mockAdminFeedbackPage(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'admin@example.com', is_admin: true },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: [] },
      }),
    });
  });
  await page.route('**/api/admin/parse-feedback**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ feedback: [] }) });
  });
}

test('admin can classify and quarantine a correction issue, then restore it', async ({ page }) => {
  await mockAdminFeedbackPage(page);

  // Issue list starts open; after the quarantine PATCH the reload returns the
  // quarantined variant. One route handler for both GET (list) and PATCH
  // (actions), since the GET has no query string when the status filter is "".
  let quarantined = false;
  const patchBodies: any[] = [];
  await page.route('**/api/admin/correction-issues**', async (route: Route) => {
    const method = route.request().method();
    if (method === 'PATCH') {
      const body = JSON.parse(route.request().postData() || '{}');
      patchBodies.push(body);
      if (body.action === 'quarantine') quarantined = true;
      if (body.action === 'restore') quarantined = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(quarantined ? QUARANTINED_ISSUE : OPEN_ISSUE),
      });
      return;
    }
    // GET list.
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ issues: [quarantined ? QUARANTINED_ISSUE : OPEN_ISSUE] }),
    });
  });

  await page.goto('/#/admin/feedback');

  const item = page.locator('#admin-issues-list .admin-feedback-item');
  await expect(item).toContainText('koira / NOUN');
  await expect(item).toContainText('3 reports');
  await expect(item).toContainText('3 distinct reporters');
  await expect(item).toContainText('Threshold candidate');

  // Quarantine without a class is blocked client-side (no PATCH sent).
  await item.locator('[data-issue-action="quarantine"]').click();
  expect(patchBodies).toHaveLength(0);

  // Classify, add a reason, quarantine.
  await item.locator('[data-issue-class="7"]').selectOption('parser_issue');
  await item.locator('[data-issue-reason="7"]').fill('parser identity wrong');
  await item.locator('[data-issue-action="quarantine"]').click();

  await expect.poll(() => patchBodies.length).toBeGreaterThan(0);
  expect(patchBodies[0]).toMatchObject({ action: 'quarantine', alpha_class: 'parser_issue', reason: 'parser identity wrong' });

  // The issue re-renders as Quarantined with a Restore action.
  await expect(item).toContainText('Quarantined');
  const restore = item.locator('[data-issue-action="restore"]');
  await expect(restore).toBeVisible();

  await restore.click();
  await expect.poll(() => patchBodies.length).toBeGreaterThan(1);
  expect(patchBodies[patchBodies.length - 1]).toMatchObject({ action: 'restore' });
});
