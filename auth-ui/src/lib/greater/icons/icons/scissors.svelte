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
	class="gr-icon gr-icon-scissors {className}"
	aria-hidden="true"
	{...restProps}
>
	<circle cx="6" cy="6" r="3"></circle><circle cx="6" cy="18" r="3"></circle><line
		x1="20"
		y1="4"
		x2="8.12"
		y2="15.88"
	></line><line x1="14.47" y1="14.48" x2="20" y2="20"></line><line
		x1="8.12"
		y1="8.12"
		x2="12"
		y2="12"
	></line>
</svg>
