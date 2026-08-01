// See https://svelte.dev/docs/kit/types#app.d.ts

declare global {
	namespace App {
		interface PageState {
			/** How many overlay entries the history stack is holding. Written by
			 *  shallow routing ($lib/overlayHistory) so the phone Back gesture
			 *  closes the top overlay instead of leaving the table. Absent on
			 *  entries no overlay ever pushed, which reads as depth 0. */
			overlayDepth?: number;
		}
	}
}

export {};
