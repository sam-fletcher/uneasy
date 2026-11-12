// Contrast, measured from the colours the browser actually resolved, so the
// styleguide's numbers are the app's numbers and cannot drift from app.css.

/** Parse `#rrggbb` and the `rgb(r, g, b)` / `rgba(...)` / `color(srgb r g b)` forms getComputedStyle returns. */
export function parseRGB(css: string): [number, number, number] | null {
	const hex = css.trim().match(/^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i);
	if (hex) return [parseInt(hex[1], 16), parseInt(hex[2], 16), parseInt(hex[3], 16)];
	const srgb = css.match(/color\(srgb\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)/);
	if (srgb) return [srgb[1], srgb[2], srgb[3]].map((n) => Math.round(parseFloat(n) * 255)) as [number, number, number];
	const rgb = css.match(/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)/);
	if (rgb) return [parseFloat(rgb[1]), parseFloat(rgb[2]), parseFloat(rgb[3])];
	return null;
}

function channel(c: number): number {
	const v = c / 255;
	return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
}

/** WCAG relative luminance of an sRGB colour. */
export function luminance([r, g, b]: [number, number, number]): number {
	return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** WCAG contrast ratio between two colours (1 to 21). */
export function contrast(a: [number, number, number], b: [number, number, number]): number {
	const la = luminance(a);
	const lb = luminance(b);
	const [hi, lo] = la >= lb ? [la, lb] : [lb, la];
	return (hi + 0.05) / (lo + 0.05);
}

/** "13.7:1" */
export function ratioLabel(ratio: number): string {
	return `${ratio.toFixed(1)}:1`;
}

/** "#e8e4d9" */
export function toHex([r, g, b]: [number, number, number]): string {
	return '#' + [r, g, b].map((n) => Math.round(n).toString(16).padStart(2, '0')).join('');
}
