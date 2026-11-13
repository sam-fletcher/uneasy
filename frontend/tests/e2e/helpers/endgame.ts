import {
  expect,
  request as pwRequest,
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  type Locator,
  type Page,
} from '@playwright/test';
import { E2E } from '../baseUrl';

// Shared setup for the specs that cover the lobby, the Shake-Up, the ending
// vote and the ended screen. Every one of them reads those screens at phone
// width, as a player would, from one browser context per player.

export interface SeededPlayer {
  id: number;
  display_name: string;
}
export interface SeededGame {
  game_id: number;
  players: SeededPlayer[];
}

/** The phone every one of these specs is read on. */
export const PHONE = { width: 375, height: 812 };

/** Seeds a game through the dev endpoint; the caller registers it for cleanup. */
export async function seedGame(body: Record<string, unknown>): Promise<SeededGame> {
  const fixture = await pwRequest.newContext({ baseURL: E2E });
  const res = await fixture.post('/api/dev/seed', { data: body });
  expect(res.ok(), `seed failed: ${res.status()} ${await res.text()}`).toBeTruthy();
  const seeded = (await res.json()) as SeededGame;
  await fixture.dispose();
  return seeded;
}

/** A browser context for one player, signed in, at phone width. */
export async function playerContext(
  browser: Browser,
  name: string,
  extra: Parameters<Browser['newContext']>[0] = {},
): Promise<BrowserContext> {
  const ctx = await browser.newContext({ baseURL: E2E, viewport: PHONE, ...extra });
  const login = await ctx.request.post(`/api/dev/login?username=${name}`);
  expect(login.ok(), `dev login as ${name} failed`).toBeTruthy();
  return ctx;
}

/** What `var(token)` resolves to, as the browser's own colour string. */
export async function resolveToken(page: Page, token: string): Promise<string> {
  return page.evaluate((t) => {
    const probe = document.createElement('div');
    probe.style.color = `var(${t})`;
    document.body.appendChild(probe);
    const value = getComputedStyle(probe).color;
    probe.remove();
    return value;
  }, token);
}

/** The painted edge and fill of one element. */
export const paint = (el: Locator) =>
  el.evaluate((node) => {
    const s = getComputedStyle(node);
    return {
      borderColor: s.borderTopColor,
      borderWidth: s.borderTopWidth,
      radius: s.borderTopLeftRadius,
      background: s.backgroundColor,
      color: s.color,
      shadow: s.boxShadow,
    };
  });

/**
 * Plays a seeded Shake-Up forward until the game ends, over the API, as the
 * players themselves: each rolls when it is their roll, and in each act every
 * token is spent on a "bump" option (they need no target), with the other
 * players letting each one stand. It is the only way to reach the `ended`
 * phase, which the seed endpoint doesn't offer.
 */
export async function playShakeUpToEnd(
  as: (name: string) => APIRequestContext,
  gameID: number,
  players: SeededPlayer[],
): Promise<void> {
  const name = (id: number) => players.find((p) => p.id === id)!.display_name;
  const any = as(players[0].display_name);
  const get = async (path: string) => {
    const res = await any.get(`/api${path}`);
    expect(res.ok(), `GET ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  const post = async (who: string, path: string, data?: unknown) => {
    const res = await as(who).post(`/api${path}`, { data });
    expect(res.ok(), `POST ${path} as ${who}: ${res.status()} ${await res.text()}`).toBeTruthy();
  };
  // Rolls hand out between 1 and 6 tokens each, three acts of three players, so
  // the loop is bounded by luck rather than by a count; this is just a backstop.
  for (let guard = 0; guard < 600; guard++) {
    const { game } = await get(`/tables/${gameID}`);
    if (game.phase === 'ended') return;
    const st = await get(`/tables/${gameID}/shake-up`);
    if (game.shake_up_step === 1) {
      await post(name(st.current_roller_id), `/rolls/${st.open_roll_id}/ready`, { is_ready: true });
    } else if (st.open_spend) {
      const sp = st.open_spend;
      for (const pid of sp.pending_reactor_ids as number[]) {
        await post(name(pid), `/tables/${gameID}/shake-up/pass`, { spend_id: sp.spend.id });
      }
      await post(name(sp.spend.player_id), `/tables/${gameID}/shake-up/commit`, { spend_id: sp.spend.id });
    } else if (st.current_actor != null) {
      const bump = (st.options as { Key: string; BumpsTrack: string }[]).find((o) => o.BumpsTrack);
      await post(name(st.current_actor), `/tables/${gameID}/shake-up/spend`, { option_key: bump!.Key });
    } else {
      // Between acts the server is still moving the table on; give it a beat.
      await new Promise((r) => setTimeout(r, 200));
    }
  }
  throw new Error('the Shake-Up did not end within the step budget');
}
