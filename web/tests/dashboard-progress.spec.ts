import { expect, test, type Page } from '@playwright/test';

// Guards the progress dashboard: the two new stat tiles (cards in review,
// reviews today) and the 14-day review-activity bar chart, which must stay
// hidden for accounts with no activity in the window.

function activityDays(counts: number[]): { day: string; count: number }[] {
  const out: { day: string; count: number }[] = [];
  const today = new Date();
  for (let i = counts.length - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setUTCDate(d.getUTCDate() - i);
    out.push({ day: d.toISOString().slice(0, 10), count: counts[counts.length - 1 - i] });
  }
  return out;
}

async function mockMe(page: Page, opts: { cardsInReview: number; reviewsToday: number; activity: number[] }): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 2, email: 'alice@example.com', is_admin: false },
        dashboard: {
          known_count:        120,
          due_count:          7,
          new_capacity_today: 20,
          cards_in_review:    opts.cardsInReview,
          reviews_today:      opts.reviewsToday,
          review_activity:    activityDays(opts.activity),
          decks:              [],
        },
      }),
    });
  });
}

test('dashboard shows cards-in-review and reviews-today tiles and the activity chart', async ({ page }) => {
  const activity = new Array(14).fill(0);
  activity[12] = 9;
  activity[13] = 4; // today
  await mockMe(page, { cardsInReview: 42, reviewsToday: 4, activity });

  await page.goto('/#/dashboard');
  await expect(page.locator('#stat-cards-in-review')).toHaveText('42');
  await expect(page.locator('#stat-reviews-today')).toHaveText('4');

  const chart = page.locator('#dashboard-activity-chart');
  await expect(page.locator('#dashboard-activity')).toBeVisible();
  await expect(chart.locator('.activity-bar-slot')).toHaveCount(14);
  // The busiest day renders the tallest bar and its count label.
  await expect(chart.locator('.activity-count').nth(12)).toHaveText('9');
});

test('activity chart stays hidden when the window has no reviews', async ({ page }) => {
  await mockMe(page, { cardsInReview: 0, reviewsToday: 0, activity: new Array(14).fill(0) });

  await page.goto('/#/dashboard');
  await expect(page.locator('#stat-cards-in-review')).toHaveText('0');
  await expect(page.locator('#dashboard-activity')).toBeHidden();
});
