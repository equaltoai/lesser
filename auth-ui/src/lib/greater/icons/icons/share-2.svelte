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
	class="gr-icon gr-icon-share-2 {className}"
	aria-hidden="true"
	{...restProps}
>
	<circle cx="18" cy="5" r="3"></circle><circle cx="6" cy="12" r="3"></circle><circle
		cx="18"
		cy="19"
		r="3"
	></circle><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"></line><line
		x1="15.41"
		y1="6.51"
		x2="8.59"
		y2="10.49"
	></line>
</svg>
