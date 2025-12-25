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
	class="gr-icon gr-icon-git-pull-request {className}"
	aria-hidden="true"
	{...restProps}
>
	<circle cx="18" cy="18" r="3"></circle><circle cx="6" cy="6" r="3"></circle><path
		d="M13 6h3a2 2 0 0 1 2 2v7"
	></path><line x1="6" y1="9" x2="6" y2="21"></line>
</svg>
