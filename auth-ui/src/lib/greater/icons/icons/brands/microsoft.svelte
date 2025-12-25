<!--
  Brand Icon - Use according to the respective company's brand guidelines.
  See the brands/index.ts file for links to official brand guidelines.
-->
<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';

	interface Props extends SVGAttributes<SVGSVGElement> {
		size?: number | string;
		color?: string;
		class?: string;
	}

	let { size = 24, color = 'currentColor', class: className = '', ...restProps }: Props = $props();
</script>

<svg
	xmlns="http://www.w3.org/2000/svg"
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill={color}
	class="gr-icon gr-icon-microsoft {className}"
	aria-hidden="true"
	{...restProps}
>
	<path d="M3 3h8v8H3V3zm10 0h8v8h-8V3zM3 13h8v8H3v-8zm10 0h8v8h-8v-8z" />
</svg>
