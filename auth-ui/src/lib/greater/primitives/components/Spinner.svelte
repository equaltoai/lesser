<!--
Spinner component - Accessible loading indicator with configurable size and color.

@component
@example
```svelte
<Spinner size="md" color="primary" label="Loading content" />

<Spinner size="lg" color="current" />
```
-->

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	/**
	 * Spinner component props interface.
	 *
	 * @public
	 */
	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'role'> {
		/**
		 * Size of the spinner.
		 * - `xs`: 12px
		 * - `sm`: 16px
		 * - `md`: 24px (default)
		 * - `lg`: 32px
		 * - `xl`: 48px
		 *
		 * @defaultValue 'md'
		 * @public
		 */
		size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';

		/**
		 * Color of the spinner.
		 * - `primary`: Uses primary theme color
		 * - `current`: Inherits color from parent (currentColor)
		 * - `white`: White color for dark backgrounds
		 * - `gray`: Neutral gray color
		 *
		 * @defaultValue 'primary'
		 * @public
		 */
		color?: 'primary' | 'current' | 'white' | 'gray';

		/**
		 * Accessible label for screen readers.
		 *
		 * @defaultValue 'Loading'
		 * @public
		 */
		label?: string;

		/**
		 * Additional CSS classes to apply.
		 *
		 * @public
		 */
		class?: string;
	}

	let {
		size = 'md',
		color = 'primary',
		label = 'Loading',
		class: className = '',
		id,
		style,
		...restProps
	}: Props = $props();

	// Size mappings in pixels
	const sizeMap: Record<string, number> = {
		xs: 12,
		sm: 16,
		md: 24,
		lg: 32,
		xl: 48,
	};

	// Compute spinner classes
	const spinnerClass = $derived(
		['gr-spinner', `gr-spinner--${size}`, `gr-spinner--${color}`, className]
			.filter(Boolean)
			.join(' ')
	);

	// Get pixel size for SVG
	const pixelSize = $derived(sizeMap[size] || sizeMap.md);
</script>

<span class={spinnerClass} role="status" aria-label={label} {id} {style} {...restProps}>
	<svg
		class="gr-spinner__svg"
		width={pixelSize}
		height={pixelSize}
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<path d="M21 12a9 9 0 11-6.219-8.56" />
	</svg>
	<span class="gr-spinner__label">{label}</span>
</span>

<style>
	.gr-spinner {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	.gr-spinner__svg {
		animation: gr-spin var(--gr-motion-duration-slow, 400ms) linear infinite;
	}

	.gr-spinner__label {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	/* Size variants */
	.gr-spinner--xs {
		width: 12px;
		height: 12px;
	}

	.gr-spinner--sm {
		width: 16px;
		height: 16px;
	}

	.gr-spinner--md {
		width: 24px;
		height: 24px;
	}

	.gr-spinner--lg {
		width: 32px;
		height: 32px;
	}

	.gr-spinner--xl {
		width: 48px;
		height: 48px;
	}

	/* Color variants */
	.gr-spinner--primary {
		color: var(--gr-semantic-action-primary-default);
	}

	.gr-spinner--current {
		color: currentColor;
	}

	.gr-spinner--white {
		color: #ffffff;
	}

	.gr-spinner--gray {
		color: var(--gr-semantic-text-secondary);
	}

	/* Reduced motion */
	@media (prefers-reduced-motion: reduce) {
		.gr-spinner__svg {
			animation: none;
		}
	}
</style>
