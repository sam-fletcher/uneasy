import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { parse } from 'svelte/compiler';

/*
 * Svelte 5 trims the whitespace at the start and end of a `{#if}` / `{#each}`
 * body. So `difficulty{#if m} by {m}{/if}` renders "difficultyby 3", and
 * `{label}{#if x} <em>(declined)</em>{/if}` renders "label(declined)". The
 * playtest audit found "vetoyour", "vetothe", "difficultyby 0", "0/ 1" and
 * "(declined)" with no space, all this one mistake.
 *
 * A body that opens or closes with a same-line space glued to inline content
 * is therefore a bug. Write the space as `{' '}` (which is an expression, not
 * trimmable): `{#if m}{' '}by {m}{/if}`. Whitespace that contains a newline is
 * indentation, which is meant to disappear, and is ignored. So is a non-breaking
 * space (`&nbsp;`), which Svelte does not trim.
 */

const SRC = join(__dirname, '..');

function svelteFiles(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...svelteFiles(p));
		else if (entry.name.endsWith('.svelte')) out.push(p);
	}
	return out;
}

type Node = { type: string; start: number; data?: string; [k: string]: unknown };
type Body = { nodes?: Node[] } | null | undefined;

const lineOf = (src: string, pos: number) => src.slice(0, pos).split('\n').length;
const endsWithWs = (n?: Node) => n?.type === 'Text' && /\s$/.test(n.data ?? '');
const startsWithWs = (n?: Node) => n?.type === 'Text' && /^\s/.test(n.data ?? '');
// ASCII blanks only: Svelte keeps U+00A0, so an `&nbsp;` is deliberate.
const sameLineWs = (run: string) => /^[ \t]+$/.test(run);

function violations(file: string): string[] {
	const src = readFileSync(file, 'utf8');
	const found: string[] = [];

	const checkBody = (body: Body, siblings: Node[], idx: number, at: number) => {
		const nodes = body?.nodes ?? [];
		if (nodes.length === 0) return;
		const first = nodes[0];
		const last = nodes[nodes.length - 1];
		const prev = siblings[idx - 1];
		const next = siblings[idx + 1];
		const where = `${relative(SRC, file)}:${lineOf(src, at)}`;
		if (first.type === 'Text' && prev && prev.type !== 'Comment' && !endsWithWs(prev)) {
			const lead = (first.data ?? '').match(/^[ \t\r\n\f\v]*/)![0];
			const onlyWs = (first.data ?? '').trim() === '';
			if (sameLineWs(lead) && (!onlyWs || nodes.length > 1)) {
				found.push(`${where}: block body starts with a trimmed space — use {' '}`);
			}
		}
		if (last.type === 'Text' && next && next.type !== 'Comment' && !startsWithWs(next)) {
			const trail = (last.data ?? '').match(/[ \t\r\n\f\v]*$/)![0];
			const onlyWs = (last.data ?? '').trim() === '';
			if (sameLineWs(trail) && (!onlyWs || nodes.length > 1)) {
				found.push(`${where}: block body ends with a trimmed space — use {' '}`);
			}
		}
	};

	const walk = (node: unknown, siblings: Node[] | null, idx: number) => {
		if (!node || typeof node !== 'object') return;
		const n = node as Node;
		if (siblings && n.type === 'IfBlock') {
			checkBody(n.consequent as Body, siblings, idx, n.start);
			checkBody(n.alternate as Body, siblings, idx, n.start);
		} else if (siblings && n.type === 'EachBlock') {
			checkBody(n.body as Body, siblings, idx, n.start);
			checkBody(n.fallback as Body, siblings, idx, n.start);
		}
		for (const key of Object.keys(n)) {
			const v = n[key];
			if (Array.isArray(v)) v.forEach((c, i) => walk(c, v as Node[], i));
			else if (v && typeof v === 'object' && 'type' in v) walk(v, null, 0);
		}
	};

	walk(parse(src, { modern: true }).fragment, null, 0);
	return found;
}

describe('Svelte block bodies keep their inline spaces', () => {
	it('no {#if}/{#each} body opens or closes with a same-line space glued to inline content', () => {
		const all = svelteFiles(SRC).flatMap(violations);
		expect(all).toEqual([]);
	});
});
