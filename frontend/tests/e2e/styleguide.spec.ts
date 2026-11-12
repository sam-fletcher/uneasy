import { test, expect, type Page } from '@playwright/test';

// The chip recipe's states, in a real browser.
//
// chip.css is checked in the unit suite by running the cascade over its rules
// (src/lib/components/shared/pieces.test.ts), but that is a model of the
// browser. This is the browser: it opens /dev/styleguide, which needs no login
// and no seeded game, hovers and locks real chips, and reads the colours the
// engine actually painted.
//
// It is the test that would have caught the first version of the recipe, where
// a hover rule out-ranked `.selected` and a hovered selected chip drew a grey
// ring around a gold inset pixel.

/** What `var(token)` resolves to, as the browser's own colour string. */
async function resolve(page: Page, token: string): Promise<string> {
	return page.evaluate((t) => {
		const probe = document.createElement('div');
		probe.style.color = `var(${t})`;
		document.body.appendChild(probe);
		const value = getComputedStyle(probe).color;
		probe.remove();
		return value;
	}, token);
}

const paint = (chip: ReturnType<Page['getByRole']>) =>
	chip.evaluate((el) => {
		const s = getComputedStyle(el);
		return { border: s.borderTopColor, color: s.color, shadow: s.boxShadow, width: s.width };
	});

test.beforeEach(async ({ page }) => {
	await page.goto('/dev/styleguide');
	await expect(page.getByRole('heading', { name: 'Chips' })).toBeVisible();
});

test('a selected chip keeps its bright gold border at rest and on hover', async ({ page }) => {
	const you = await resolve(page, '--color-you');
	const aid = page.getByRole('button', { name: 'Aid', exact: true });
	await expect(aid).toHaveAttribute('aria-pressed', 'true');

	const rest = await paint(aid);
	expect(rest.border).toBe(you);
	expect(rest.color).toBe(you);
	expect(rest.shadow).toContain(you); // the second pixel of the 2px border

	await aid.hover();
	const hovered = await paint(aid);
	expect(hovered.border, 'hovering must not repaint a selected chip’s border').toBe(you);
	expect(hovered.shadow).toContain(you);
});

test('an unselected chip lightens its border on hover', async ({ page }) => {
	const interfere = page.getByRole('button', { name: 'Interfere', exact: true });
	const rest = await paint(interfere);
	expect(rest.border).toBe(await resolve(page, '--color-border-control'));

	await interfere.hover();
	expect((await paint(interfere)).border).toBe(await resolve(page, '--color-border-control-hover'));
});

test('a chosen chip that is locked keeps the gold border and takes faint text', async ({ page }) => {
	const locked = page.getByRole('button', { name: 'Chosen, locked' });
	await expect(locked).toBeDisabled();
	await locked.hover({ force: true });
	const p = await paint(locked);
	expect(p.border).toBe(await resolve(page, '--color-you'));
	expect(p.color).toBe(await resolve(page, '--color-text-faint'));
});

test('choosing a chip never resizes it', async ({ page }) => {
	// The second border pixel is an inset shadow so nothing moves. Compare the
	// chip's laid-out width before and after it becomes the selected one.
	const interfere = page.getByRole('button', { name: 'Interfere', exact: true });
	const before = (await interfere.boundingBox())!;
	await interfere.click();
	await expect(interfere).toHaveAttribute('aria-pressed', 'true');
	const after = (await interfere.boundingBox())!;
	expect(after.width).toBeCloseTo(before.width, 1);
	expect(after.height).toBeCloseTo(before.height, 1);
});
