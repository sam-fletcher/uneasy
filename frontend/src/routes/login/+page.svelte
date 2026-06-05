<!-- Legacy route: the combined entry screen now lives at `/`. Redirect, keeping ?next=. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { page } from '$app/state';

	onMount(() => {
		const next = page.url.searchParams.get('next');
		goto(`/${next ? `?next=${encodeURIComponent(next)}` : ''}`, { replaceState: true });
	});
</script>
