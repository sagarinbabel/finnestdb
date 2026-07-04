import { test, expect, type Page } from '@playwright/test';
async function mockAnonMe(page: Page) {
  await page.route('**/api/me', r => r.fulfill({ status:200, contentType:'application/json',
    body: JSON.stringify({ authenticated:false, user:null, anon_max_chars:300000 }) }));
}
// The theme picker lives in .nav-actions, which (like the rest of the desktop
// nav) collapses under the hamburger at <=760px — pre-existing behaviour the
// old single toggle shared. To verify the *skin rendering* at mobile width we
// seed the persisted choice directly and reload, exactly as a returning user
// who set the theme on desktop would experience it.
async function pick(page: Page, skin: string, mode: string) {
  await page.evaluate(([s, m]) => {
    localStorage.setItem('skin', s);
    localStorage.setItem('theme', m);
  }, [skin, mode]);
  await page.reload();
}
function styles(page: Page, sel: string, props: string[]) {
  return page.evaluate(([sel, props]) => {
    const el = document.querySelector(sel); if (!el) return null;
    const cs = getComputedStyle(el);
    const out: Record<string,string> = {};
    for (const p of props) out[p] = cs.getPropertyValue(p).trim();
    return out;
  }, [sel, props] as [string, string[]]);
}
for (const w of [1280, 375]) {
  for (const [skin, mode, label] of [['aalto','light','Paimio'],['aalto','dark','Sanatorium'],['ink','light','Ink-Light']]) {
    test(`[${w}px] ${label}: tokens + fonts + no horizontal overflow`, async ({ page }) => {
      await page.setViewportSize({ width: w, height: 900 });
      await mockAnonMe(page);
      await page.goto('/');
      await pick(page, skin, mode);
      // font-family under aalto must be the prototype trio
      const hero = await styles(page, '.hero-title', ['font-family','color']);
      const body = await styles(page, 'body', ['background-color','color']);
      const tokens = await page.evaluate(() => {
        const cs = getComputedStyle(document.documentElement);
        return {
          fontDisp: cs.getPropertyValue('--font-disp').trim(),
          fontSans: cs.getPropertyValue('--font-sans').trim(),
          known: cs.getPropertyValue('--known').trim(),
          learning: cs.getPropertyValue('--learning').trim(),
          neu: cs.getPropertyValue('--new').trim(),
          accent: cs.getPropertyValue('--accent').trim(),
        };
      });
      // Every skin defines the word-status tokens.
      expect(tokens.known).not.toBe('');
      expect(tokens.learning).not.toBe('');
      expect(tokens.neu).not.toBe('');
      if (skin === 'aalto') {
        expect(tokens.fontDisp).toContain('Newsreader');
        expect(tokens.fontSans).toContain('Inter Tight');
      } else {
        expect(tokens.fontDisp).toContain('Fraunces');
      }
      // No horizontal scroll at this width (mobile fixes must hold).
      const overflow = await page.evaluate(() =>
        document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow, `horizontal overflow px @${w}`).toBeLessThanOrEqual(1);
      console.log(`[${w}] ${label}`, JSON.stringify({ ...tokens, heroFont: hero?.['font-family'], bodyBg: body?.['background-color'] }));
    });
  }
}
