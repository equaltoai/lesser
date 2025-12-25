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
	class="gr-icon gr-icon-instagram {className}"
	aria-hidden="true"
	{...restProps}
>
	<rect x="2" y="2" width="20" height="20" rx="5" ry="5"></rect><path
		d="M16 11.37A4 4 0 1 1 12.63 8 4 4 0 0 1 16 11.37z"
	></path><line x1="17.5" y1="6.5" x2="17.51" y2="6.5"></line>
</svg>
