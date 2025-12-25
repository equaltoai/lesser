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
	class="gr-icon gr-icon-sunset {className}"
	aria-hidden="true"
	{...restProps}
>
	<path d="M17 18a5 5 0 0 0-10 0"></path><line x1="12" y1="9" x2="12" y2="2"></line><line
		x1="4.22"
		y1="10.22"
		x2="5.64"
		y2="11.64"
	></line><line x1="1" y1="18" x2="3" y2="18"></line><line x1="21" y1="18" x2="23" y2="18"
	></line><line x1="18.36" y1="11.64" x2="19.78" y2="10.22"></line><line
		x1="23"
		y1="22"
		x2="1"
		y2="22"
	></line><polyline points="16 5 12 9 8 5"></polyline>
</svg>
