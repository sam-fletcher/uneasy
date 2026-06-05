<!-- Legacy route: the combined entry screen now lives at `/`. Redirect into sign-up mode, keeping ?next=. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { page } from '$app/state';

	onMount(() => {
		const next = page.url.searchParams.get('next');
		const params = new URLSearchParams({ mode: 'signup' });
		if (next) params.set('next', next);
		goto(`/?${params.toString()}`, { replaceState: true });
	});
</script>
