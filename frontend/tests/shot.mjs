// tests/shot.mjs — screenshot a table as a chosen player, in an isolated
// browser context.
//
// Why this exists rather than "just use the browser":
//
// The session cookie is per-origin, so a single browser is a single player.
// Looking at what *another* player sees means logging in again, which
// replaces the cookie for every tab at once. That makes the obvious approach
// — one shared browser — both serial and fragile: two testers (or two agents)
// working at the same time silently become each other.
//
// Each run here launches its own Chromium with its own context and its own
// cookie jar, so any number can run at once and "what does bob see while
// alice is mid-sub-flow?" is one more invocation rather than a dance.
//
//   node tests/shot.mjs --game 331 --as alice --label sp-01-mar-options
//   node tests/shot.mjs --game 331 --as bob   --label sp-01-bob-view --width 1280
//   node tests/shot.mjs --game 331 --as carol --text-only
//
// Writes <out>/<label>.png and prints the page's visible text plus any
// console errors — the text is usually enough to tell which state you landed
// in, so a caller can skip looking at the image until it matters.

import { chromium } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
import path from 'node:path';

function parseArgs(argv) {
  const out = {
    base: 'http://localhost:8080',
    width: 375,
    height: 812,
    out: 'shots',
    wait: 1200,
    label: null,
    game: null,
    as: null,
    textOnly: false,
    path: null,
    clicks: [],
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => argv[++i];
    switch (a) {
      case '--game': out.game = next(); break;
      case '--as': out.as = next(); break;
      case '--label': out.label = next(); break;
      case '--base': out.base = next(); break;
      case '--out': out.out = next(); break;
      case '--width': out.width = Number(next()); break;
      case '--height': out.height = Number(next()); break;
      case '--wait': out.wait = Number(next()); break;
      case '--path': out.path = next(); break;   // a route other than /table/<id>
      case '--text-only': out.textOnly = true; break;
      case '--click': out.clicks.push(next()); break;
      default:
        throw new Error(`unknown flag ${a}`);
    }
  }
  if (!out.as) throw new Error('--as <username> is required');
  if (!out.game && !out.path) throw new Error('--game <id> (or --path) is required');
  if (!out.label) out.label = `${out.as}-${out.game ?? 'page'}-${out.width}`;
  return out;
}

const opts = parseArgs(process.argv.slice(2));
const target = opts.path ?? `/table/${opts.game}`;

const browser = await chromium.launch();
const context = await browser.newContext({
  viewport: { width: opts.width, height: opts.height },
  // Below 768 the app is meant to be read as a phone; matching the device
  // flags means load-time gates see what a phone would.
  isMobile: opts.width < 768,
  hasTouch: opts.width < 768,
});

const consoleErrors = [];
const pageErrors = [];

try {
  // context.request shares this context's cookie jar, so the dev login lands
  // on the pages we open below.
  const login = await context.request.post(
    `${opts.base}/api/dev/login?username=${encodeURIComponent(opts.as)}`);
  if (!login.ok()) {
    throw new Error(`dev login as ${opts.as} failed: ${login.status()} ${await login.text()}`);
  }

  const page = await context.newPage();
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('pageerror', (e) => pageErrors.push(String(e)));

  await page.goto(opts.base + target, { waitUntil: 'domcontentloaded' });
  // The SPA renders, then fetches game state over several calls; networkidle
  // covers the fetches and the extra wait covers the render after them.
  await page.waitForLoadState('networkidle').catch(() => {});
  await page.waitForTimeout(opts.wait);

  // --click is for client-side UI state only: expanding a section, opening an
  // overlay. Anything the server records should be driven over the API
  // instead, because each run starts from a fresh context and would replay
  // the whole click sequence — and re-submit it.
  for (const label of opts.clicks) {
    const target = page.getByText(label, { exact: false }).first();
    await target.click({ timeout: 5000 });
    await page.waitForTimeout(400);
  }

  let shotPath = null;
  if (!opts.textOnly) {
    await mkdir(opts.out, { recursive: true });
    shotPath = path.join(opts.out, `${opts.label}.png`);
    await page.screenshot({ path: shotPath, fullPage: true });
  }

  const text = await page.locator('body').innerText();

  console.log(`── ${opts.label} · as ${opts.as} · ${opts.width}px ──`);
  if (shotPath) console.log(`image: ${path.resolve(shotPath)}`);
  console.log('\n--- visible text ---');
  console.log(text.trim());
  if (consoleErrors.length || pageErrors.length) {
    console.log('\n--- console errors ---');
    for (const e of [...pageErrors, ...consoleErrors]) console.log(`  ${e}`);
  }
} finally {
  await context.close();
  await browser.close();
}
