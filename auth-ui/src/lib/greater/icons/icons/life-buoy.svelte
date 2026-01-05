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
	class="gr-icon gr-icon-life-buoy {className}"
	aria-hidden="true"
	{...restProps}
>
	<circle cx="12" cy="12" r="10"></circle><circle cx="12" cy="12" r="4"></circle><line
		x1="4.93"
		y1="4.93"
		x2="9.17"
		y2="9.17"
	></line><line x1="14.83" y1="14.83" x2="19.07" y2="19.07"></line><line
		x1="14.83"
		y1="9.17"
		x2="19.07"
		y2="4.93"
	></line><line x1="14.83" y1="9.17" x2="18.36" y2="5.64"></line><line
		x1="4.93"
		y1="19.07"
		x2="9.17"
		y2="14.83"
	></line>
</svg>
