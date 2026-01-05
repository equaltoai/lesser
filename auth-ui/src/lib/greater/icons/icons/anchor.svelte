<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';

	interface Props extends SVGAttributes<SVGSVGElement> {
		size?: number | string;
		color?: string;
		strokeWidth?: number | string;
		class?: string;
	}

	let {
		size = 24,
		color = 'currentColor',
		strokeWidth = 2,
		class: className = '',
		...restProps
	}: Props = $props();
</script>

<svg
	xmlns="http://www.w3.org/2000/svg"
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke={color}
	stroke-width={strokeWidth}
	stroke-linecap="round"
	stroke-linejoin="round"
	class="gr-icon gr-icon-anchor {className}"
	aria-hidden="true"
	{...restProps}
>
	<circle cx="12" cy="5" r="3"></circle><line x1="12" y1="22" x2="12" y2="8"></line><path
		d="M5 12H2a10 10 0 0 0 20 0h-3"
	></path>
</svg>
