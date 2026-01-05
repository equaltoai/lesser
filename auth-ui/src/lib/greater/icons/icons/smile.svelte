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
	class="gr-icon gr-icon-smile {className}"
	aria-hidden="true"
	{...restProps}
>
	<circle cx="12" cy="12" r="10"></circle><path d="M8 14s1.5 2 4 2 4-2 4-2"></path><line
		x1="9"
		y1="9"
		x2="9.01"
		y2="9"
	></line><line x1="15" y1="9" x2="15.01" y2="9"></line>
</svg>
