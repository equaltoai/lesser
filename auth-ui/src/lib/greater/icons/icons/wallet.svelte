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
	class="gr-icon gr-icon-wallet {className}"
	aria-hidden="true"
	{...restProps}
>
	<path d="M4 7V5a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v2" /><rect
		x="2"
		y="7"
		width="20"
		height="14"
		rx="2"
		ry="2"
	></rect><path d="M22 12h-6a2 2 0 0 0 0 4h6"></path><circle cx="16" cy="14" r="1"></circle>
</svg>
