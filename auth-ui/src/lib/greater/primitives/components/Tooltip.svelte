<svelte:options
	customElement={{
		props: {
			content: {},
			placement: {},
			trigger: {},
			delay: {},
			disabled: {},
			class: {},
			children: {},
		},
	}}
/>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { untrack } from 'svelte';
	import { useStableId } from 'src/lib/greater/utils';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
		content: string;
		id?: string;
		placement?: 'top' | 'bottom' | 'left' | 'right' | 'auto';
		trigger?: 'hover' | 'focus' | 'click' | 'manual';
		delay?: { show?: number; hide?: number } | number;
		disabled?: boolean;
		class?: string;
		children: Snippet;
	}

	let {
		content,
		id,
		placement = 'top',
		trigger = 'hover',
		delay = { show: 500, hide: 100 },
		disabled = false,
		class: className = '',
		children,
		...restProps
	}: Props = $props<Props>();

	// Normalize delay prop
	const normalizedDelay = $derived(() => {
		if (typeof delay === 'number') {
			return { show: delay, hide: delay };
		}
		return { show: 500, hide: 100, ...delay };
	});

	// State management
	let isVisible = $state(false);
	let triggerElement: HTMLElement | null = $state(null);
	let tooltipElement: HTMLDivElement | null = $state(null);
	let showTimeout: ReturnType<typeof setTimeout> | null = $state(null);
	let hideTimeout: ReturnType<typeof setTimeout> | null = $state(null);
	let longPressTimeout: ReturnType<typeof setTimeout> | null = $state(null);
	let actualPlacement = $state(untrack(() => placement));

	$effect(() => {
		actualPlacement = placement;
	});

	// Computed tooltip position
	let tooltipPosition = $state({ top: 0, left: 0 });

	function clearTimeouts() {
		if (showTimeout) {
			clearTimeout(showTimeout);
			showTimeout = null;
		}
		if (hideTimeout) {
			clearTimeout(hideTimeout);
			hideTimeout = null;
		}
		if (longPressTimeout) {
			clearTimeout(longPressTimeout);
			longPressTimeout = null;
		}
	}

	function show() {
		if (disabled || isVisible) return;

		clearTimeouts();
		showTimeout = setTimeout(() => {
			isVisible = true;
			// Calculate position after tooltip is rendered
			requestAnimationFrame(() => {
				calculatePosition();
			});
		}, normalizedDelay().show);
	}

	function hide() {
		if (disabled || !isVisible) return;

		clearTimeouts();
		hideTimeout = setTimeout(() => {
			isVisible = false;
		}, normalizedDelay().hide);
	}

	function calculatePosition() {
		if (!triggerElement || !tooltipElement) return;

		const triggerRect = triggerElement.getBoundingClientRect();
		const tooltipRect = tooltipElement.getBoundingClientRect();
		const viewportWidth = window.innerWidth;
		const viewportHeight = window.innerHeight;
		const scrollX = window.scrollX;
		const scrollY = window.scrollY;

		let top = 0;
		let left = 0;
		let finalPlacement = placement;

		// Calculate preferred position
		switch (placement) {
			case 'top':
				top = triggerRect.top - tooltipRect.height - 8;
				left = triggerRect.left + (triggerRect.width - tooltipRect.width) / 2;
				break;
			case 'bottom':
				top = triggerRect.bottom + 8;
				left = triggerRect.left + (triggerRect.width - tooltipRect.width) / 2;
				break;
			case 'left':
				top = triggerRect.top + (triggerRect.height - tooltipRect.height) / 2;
				left = triggerRect.left - tooltipRect.width - 8;
				break;
			case 'right':
				top = triggerRect.top + (triggerRect.height - tooltipRect.height) / 2;
				left = triggerRect.right + 8;
				break;
			case 'auto': {
				// Smart placement - find the best position
				const positions = [
					{
						placement: 'top',
						top: triggerRect.top - tooltipRect.height - 8,
						left: triggerRect.left + (triggerRect.width - tooltipRect.width) / 2,
					},
					{
						placement: 'bottom',
						top: triggerRect.bottom + 8,
						left: triggerRect.left + (triggerRect.width - tooltipRect.width) / 2,
					},
					{
						placement: 'left',
						top: triggerRect.top + (triggerRect.height - tooltipRect.height) / 2,
						left: triggerRect.left - tooltipRect.width - 8,
					},
					{
						placement: 'right',
						top: triggerRect.top + (triggerRect.height - tooltipRect.height) / 2,
						left: triggerRect.right + 8,
					},
				];

				// Find position that fits best in viewport
				const bestPosition =
					positions.find(
						(pos) =>
							pos.top >= 0 &&
							pos.top + tooltipRect.height <= viewportHeight &&
							pos.left >= 0 &&
							pos.left + tooltipRect.width <= viewportWidth
					) || positions[0]; // Fallback to top

				top = bestPosition.top;
				left = bestPosition.left;
				finalPlacement = bestPosition.placement as typeof placement;
				break;
			}
		}

		// Clamp to viewport bounds
		left = Math.max(8, Math.min(left, viewportWidth - tooltipRect.width - 8));
		top = Math.max(8, Math.min(top, viewportHeight - tooltipRect.height - 8));

		// Add scroll offset
		top += scrollY;
		left += scrollX;

		tooltipPosition = { top, left };
		actualPlacement = finalPlacement;
	}

	function handleMouseEnter() {
		if (trigger === 'hover' || trigger === 'click') {
			show();
		}
	}

	function handleMouseLeave() {
		if (trigger === 'hover') {
			hide();
		}
	}

	function handleFocus() {
		if (trigger === 'focus') {
			show();
		}
	}

	function handleBlur() {
		if (trigger === 'focus') {
			hide();
		}
	}

	function handleClick() {
		if (trigger === 'click') {
			if (isVisible) {
				hide();
			} else {
				show();
			}
		}
	}

	function handleTouchStart() {
		// Long press for mobile
		longPressTimeout = setTimeout(() => {
			show();
		}, 500);
	}

	function handleTouchEnd() {
		if (longPressTimeout) {
			clearTimeout(longPressTimeout);
			longPressTimeout = null;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && isVisible) {
			hide();
		}
	}

	function handleClickOutside(event: MouseEvent) {
		if (
			trigger === 'click' &&
			isVisible &&
			tooltipElement &&
			!tooltipElement.contains(event.target as Node) &&
			triggerElement &&
			!triggerElement.contains(event.target as Node)
		) {
			hide();
		}
	}

	// Window resize handler
	function handleResize() {
		if (isVisible) {
			calculatePosition();
		}
	}

	// Effects
	$effect(() => {
		if (isVisible) {
			document.addEventListener('click', handleClickOutside);
			window.addEventListener('resize', handleResize);
			window.addEventListener('scroll', calculatePosition);

			return () => {
				document.removeEventListener('click', handleClickOutside);
				window.removeEventListener('resize', handleResize);
				window.removeEventListener('scroll', calculatePosition);
			};
		}
	});

	// Cleanup on unmount
	$effect(() => {
		return () => {
			clearTimeouts();
		};
	});

	// Compute tooltip classes
	const tooltipClass = $derived(() => {
		const classes = [
			'gr-tooltip',
			`gr-tooltip--${actualPlacement}`,
			isVisible && 'gr-tooltip--visible',
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	// Generate unique ID for accessibility
	const stableId = useStableId('tooltip');
	const tooltipId = $derived(id || stableId.value || undefined);
</script>

<div class="gr-tooltip-container">
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		bind:this={triggerElement}
		class="gr-tooltip-trigger"
		aria-describedby={isVisible ? tooltipId : undefined}
		onmouseenter={handleMouseEnter}
		onmouseleave={handleMouseLeave}
		onfocusin={handleFocus}
		onfocusout={handleBlur}
		onclick={handleClick}
		ontouchstart={handleTouchStart}
		ontouchend={handleTouchEnd}
		onkeydown={handleKeydown}
		role={trigger === 'click' ? 'button' : 'presentation'}
		tabindex={trigger === 'click' ? 0 : undefined}
	>
		{@render children()}
	</div>

	{#if isVisible}
		<div
			bind:this={tooltipElement}
			class={tooltipClass()}
			id={tooltipId}
			role="tooltip"
			style="position: absolute; top: {tooltipPosition.top}px; left: {tooltipPosition.left}px; z-index: 9999;"
			{...restProps}
		>
			<div class="gr-tooltip__content">
				{content}
			</div>
			<div class="gr-tooltip__arrow" aria-hidden="true"></div>
		</div>
	{/if}
</div>
