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
	class="gr-icon gr-icon-align-right {className}"
	aria-hidden="true"
	{...restProps}
>
	<line x1="21" y1="10" x2="7" y2="10"></line><line x1="21" y1="6" x2="3" y2="6"></line><line
		x1="21"
		y1="14"
		x2="3"
		y2="14"
	></line><line x1="21" y1="18" x2="7" y2="18"></line>
</svg>
