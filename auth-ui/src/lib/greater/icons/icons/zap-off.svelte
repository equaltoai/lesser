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
	class="gr-icon gr-icon-zap-off {className}"
	aria-hidden="true"
	{...restProps}
>
	<polyline points="12.41 6.75 13 2 10.57 4.92"></polyline><polyline
		points="18.57 12.91 21 10 15.66 10"
	></polyline><polyline points="8 8 3 14 12 14 11 22 16 16"></polyline><line
		x1="1"
		y1="1"
		x2="23"
		y2="23"
	></line>
</svg>
