import { expect, test, type Page } from '@playwright/test';

// Account deletion flow (Languages page → Account section).
//
// The backend (DELETE /api/me) cascades all user data, clears the session
// cookie, and is protected by the same-origin state-changing request guard
// (Origin/Referer must match the request host). These tests mock the API and
// assert the client side of the contract: a deliberate confirm dialog, a safe
// cancel path, the DELETE firing with same-origin guard headers, and the
// signed-out landing on success.

interface DeleteCapture {
  count: number;
  origin: string;
  referer: string;
}

// Mocks GET /api/me as a signed-in user and captures DELETE /api/me.
// `deleteStatus` controls the DELETE response so the failure path can be
// exercised with the same mock.
async function mockMeWithDelete(
  page: Page,
  capture: DeleteCapture,
  deleteStatus = 200,
): Promise<void> {
  await page.route('**/api/me', async (route) => {
    const request = route.request();
    if (request.method() === 'DELETE') {
      capture.count += 1;
      capture.origin = request.headers()['origin'] ?? '';
      capture.referer = request.headers()['referer'] ?? '';
      if (deleteStatus === 200) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ status: 'deleted' }),
        });
      } else {
        await route.fulfill({ status: deleteStatus, body: 'Database error' });
      }
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: [] },
        languages: {
          learning: ['FI', 'ET'],
          active: 'FI',
          stats: { FI: { decks: 0, known_words: 0 }, ET: { decks: 0, known_words: 0 } },
        },
      }),
    });
  });
}

test.describe('Account deletion', () => {
  test('requires confirm, sends guarded DELETE /api/me, lands signed out', async ({ page }) => {
    const capture: DeleteCapture = { count: 0, origin: '', referer: '' };
    await mockMeWithDelete(page, capture);

    await page.goto('/#/languages');
    await expect(page.locator('#languages-page')).toHaveClass(/active/);

    const deleteBtn = page.locator('#account-delete');
    await expect(deleteBtn).toBeVisible();
    // The tooltip wording is the contract: the button itself never deletes.
    await expect(deleteBtn).toHaveAttribute('data-tooltip', /confirm deletion/i);

    // Click → confirmation dialog spells out what is deleted and that it is
    // permanent.
    await deleteBtn.click();
    const dialog = page.locator('#dialog-modal');
    await expect(dialog).not.toHaveClass(/hidden/);
    await expect(dialog).toContainText(/Delete your account\?/i);
    await expect(dialog).toContainText('alice@example.com');
    await expect(dialog).toContainText(/decks/i);
    await expect(dialog).toContainText(/review history/i);
    await expect(dialog).toContainText(/known/i);
    await expect(dialog).toContainText(/cannot be undone/i);

    // Cancel path: dialog dismisses, no DELETE fires, still signed in on the
    // Languages page.
    await page.locator('#dialog-modal-cancel').click();
    await expect(dialog).toHaveClass(/hidden/);
    expect(capture.count).toBe(0);
    await expect(page.locator('#languages-page')).toHaveClass(/active/);

    // Confirm path: DELETE /api/me fires with the same-origin guard headers
    // the backend's allowStateChangingRequest checks (Origin/Referer).
    await deleteBtn.click();
    await expect(dialog).not.toHaveClass(/hidden/);
    await page.locator('#dialog-modal-confirm').click();
    await expect.poll(() => capture.count).toBe(1);
    const expectedOrigin = new URL(page.url()).origin;
    expect(
      capture.origin === expectedOrigin || capture.referer.startsWith(expectedOrigin),
      `DELETE must carry a same-origin Origin or Referer header (origin=${capture.origin}, referer=${capture.referer})`,
    ).toBe(true);

    // Success: client state cleared, toast shown, routed to the signed-out
    // landing page.
    await expect(page.locator('body')).toHaveAttribute('data-auth-state', 'anon');
    await expect(page.locator('#landing-page')).toHaveClass(/active/);
    await expect(page.locator('.toast')).toContainText(/account and data have been deleted/i);
  });

  test('failed DELETE shows an error toast and leaves the app usable', async ({ page }) => {
    const capture: DeleteCapture = { count: 0, origin: '', referer: '' };
    await mockMeWithDelete(page, capture, 500);

    await page.goto('/#/languages');
    const deleteBtn = page.locator('#account-delete');
    await deleteBtn.click();
    await expect(page.locator('#dialog-modal')).not.toHaveClass(/hidden/);
    await page.locator('#dialog-modal-confirm').click();
    await expect.poll(() => capture.count).toBe(1);

    // Standard error toast, dialog closed, still signed in on the Languages
    // page with the button re-enabled for a retry.
    await expect(page.locator('.toast.error')).toBeVisible();
    await expect(page.locator('#dialog-modal')).toHaveClass(/hidden/);
    await expect(page.locator('body')).toHaveAttribute('data-auth-state', 'user');
    await expect(page.locator('#languages-page')).toHaveClass(/active/);
    await expect(deleteBtn).toBeEnabled();
  });
});
