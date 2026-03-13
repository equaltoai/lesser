<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { untrack } from 'svelte';
	import { useStableId } from 'src/lib/greater/utils';

	export type Placement = 'top' | 'bottom' | 'left' | 'right' | 'auto';
	type ActualPlacement = Exclude<Placement, 'auto'>;

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
		content: string;
		id?: string;
		placement?: Placement;
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
		style: _style,
		...restProps
	}: Props = $props();

	// Normalize delay prop
	const normalizedDelay = $derived.by(() => {
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
	let actualPlacement: ActualPlacement = $state(
		untrack(() => (placement === 'auto' ? 'top' : placement))
	);

	$effect(() => {
		if (placement !== 'auto') {
			actualPlacement = placement;
		}
	});

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

	/**
	 * Calculate the best placement for auto placement mode.
	 * Uses viewport heuristics to select the best placement class.
	 * Returns a placement string (no pixel calculations).
	 */
	function calculatePlacement(): ActualPlacement {
		if (placement !== 'auto') return placement;
		if (!triggerElement) return 'top';

		const triggerRect = triggerElement.getBoundingClientRect();
		const viewportHeight = window.innerHeight;
		const viewportWidth = window.innerWidth;

		// Space available in each direction
		const spaceAbove = triggerRect.top;
		const spaceBelow = viewportHeight - triggerRect.bottom;
		const spaceLeft = triggerRect.left;
		const spaceRight = viewportWidth - triggerRect.right;

		// Thresholds for minimum space needed
		const verticalThreshold = 100;
		const horizontalThreshold = 150;

		// Prefer top, then bottom, then right, then left
		if (spaceAbove >= verticalThreshold) return 'top';
		if (spaceBelow >= verticalThreshold) return 'bottom';
		if (spaceRight >= horizontalThreshold) return 'right';
		if (spaceLeft >= horizontalThreshold) return 'left';

		// Fallback: choose the direction with most space
		const maxSpace = Math.max(spaceAbove, spaceBelow, spaceLeft, spaceRight);
		if (maxSpace === spaceAbove) return 'top';
		if (maxSpace === spaceBelow) return 'bottom';
		if (maxSpace === spaceRight) return 'right';
		return 'left';
	}

	function show() {
		if (disabled || isVisible) return;

		clearTimeouts();
		showTimeout = setTimeout(() => {
			// Calculate placement before showing (for auto placement)
			if (placement === 'auto') {
				actualPlacement = calculatePlacement();
			}
			isVisible = true;
		}, normalizedDelay.show);
	}

	function hide() {
		if (disabled || !isVisible) return;

		clearTimeouts();
		hideTimeout = setTimeout(() => {
			isVisible = false;
		}, normalizedDelay.hide);
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

	// Window resize handler - recalculate placement for auto mode
	function handleResize() {
		if (isVisible && placement === 'auto') {
			actualPlacement = calculatePlacement();
		}
	}

	// Effects
	$effect(() => {
		if (!isVisible) return;

		document.addEventListener('click', handleClickOutside);
		window.addEventListener('resize', handleResize);
		window.addEventListener('scroll', handleResize);

		return () => {
			document.removeEventListener('click', handleClickOutside);
			window.removeEventListener('resize', handleResize);
			window.removeEventListener('scroll', handleResize);
		};
	});

	// Cleanup on unmount
	$effect(() => {
		return () => {
			clearTimeouts();
		};
	});

	// Compute tooltip classes - CSP compliant (no inline styles)
	const tooltipClass = $derived.by(() => {
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
			class={tooltipClass}
			id={tooltipId}
			role="tooltip"
			{...restProps}
		>
			<div class="gr-tooltip__content">
				{content}
			</div>
			<div class="gr-tooltip__arrow" aria-hidden="true"></div>
		</div>
	{/if}
</div>
