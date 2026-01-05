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
	class="gr-icon gr-icon-building {className}"
	aria-hidden="true"
	{...restProps}
>
	<rect x="3" y="7" width="7" height="15" rx="1" /><rect
		x="14"
		y="3"
		width="7"
		height="19"
		rx="1"
	/><line x1="6" y1="11" x2="6" y2="11.01" /><line x1="6" y1="15" x2="6" y2="15.01" /><line
		x1="17.5"
		y1="7"
		x2="17.5"
		y2="7.01"
	/><line x1="17.5" y1="11" x2="17.5" y2="11.01" /><line
		x1="17.5"
		y1="15"
		x2="17.5"
		y2="15.01"
	/><line x1="17.5" y1="19" x2="17.5" y2="19.01" />
</svg>
