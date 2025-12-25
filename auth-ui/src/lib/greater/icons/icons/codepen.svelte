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
	class="gr-icon gr-icon-codepen {className}"
	aria-hidden="true"
	{...restProps}
>
	<polygon points="12 2 22 8.5 22 15.5 12 22 2 15.5 2 8.5 12 2"></polygon><line
		x1="12"
		y1="22"
		x2="12"
		y2="15.5"
	></line><polyline points="22 8.5 12 15.5 2 8.5"></polyline><polyline
		points="2 15.5 12 8.5 22 15.5"
	></polyline><line x1="12" y1="2" x2="12" y2="8.5"></line>
</svg>
