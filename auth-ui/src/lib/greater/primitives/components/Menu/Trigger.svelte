<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { getMenuContext } from './context.svelte';

	const MANAGED_ATTR = 'data-gr-menu-trigger-managed';
	const focusableSelector =
		'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		/** Custom CSS class */
		class?: string;
		/** Trigger content */
		children: Snippet;
		/** Whether trigger is disabled */
		disabled?: boolean;
	}

	let { class: className = '', children, disabled = false, ...restProps }: Props = $props();

	const ctx = getMenuContext();
	let triggerRef: HTMLElement | null = $state(null);
	let interactiveElement: HTMLElement | null = $state(null);
	let managedElement: HTMLElement | null = null;
	let mutationObserver: MutationObserver | null = null;

	function handleClick(event: MouseEvent) {
		if (disabled) return;
		event.preventDefault();
		ctx.toggle();
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (disabled) return;

		switch (event.key) {
			case 'ArrowDown':
			case 'Enter':
			case ' ':
				event.preventDefault();
				ctx.open();
				break;
			case 'ArrowUp':
				event.preventDefault();
				ctx.open();
				// Focus last item when opening with ArrowUp
				if (ctx.items.length > 0) {
					const lastEnabled = [...ctx.items].reverse().findIndex((item) => !item.disabled);
					if (lastEnabled !== -1) {
						ctx.setActiveIndex(ctx.items.length - 1 - lastEnabled);
					}
				}
				break;
			case 'Escape':
				if (ctx.isOpen) {
					event.preventDefault();
					ctx.close();
				}
				break;
		}
	}

	function findInteractiveElement() {
		if (!triggerRef) return null;
		const focusable = triggerRef.querySelector(focusableSelector) as HTMLElement | null;
		return focusable ?? triggerRef;
	}

	function isNativeInteractive(element: HTMLElement) {
		if (
			element instanceof HTMLButtonElement ||
			element instanceof HTMLInputElement ||
			element instanceof HTMLSelectElement ||
			element instanceof HTMLTextAreaElement
		) {
			return true;
		}

		return element instanceof HTMLAnchorElement && element.hasAttribute('href');
	}

	function cleanupManaged(target: HTMLElement | null) {
		if (!target || target.getAttribute(MANAGED_ATTR) !== 'true') return;
		target.removeAttribute('aria-haspopup');
		target.removeAttribute('aria-controls');
		target.removeAttribute('aria-expanded');
		target.removeAttribute('aria-disabled');
		if (!isNativeInteractive(target)) {
			target.removeAttribute('role');
			target.removeAttribute('tabindex');
		}
		target.removeAttribute('id');
		target.removeAttribute(MANAGED_ATTR);
	}

	function applyAttributes(target: HTMLElement | null) {
		if (!target) return;
		target.setAttribute(MANAGED_ATTR, 'true');
		target.setAttribute('aria-haspopup', 'menu');
		target.setAttribute('aria-controls', ctx.menuId);
		target.setAttribute('aria-expanded', ctx.isOpen ? 'true' : 'false');
		target.id = ctx.triggerId;

		if (disabled) {
			target.setAttribute('aria-disabled', 'true');
		} else {
			target.removeAttribute('aria-disabled');
		}

		if (isNativeInteractive(target)) {
			target.removeAttribute('role');
			if (target instanceof HTMLButtonElement) {
				target.disabled = disabled;
			}
		} else {
			target.setAttribute('role', 'button');
			target.tabIndex = disabled ? -1 : 0;
		}
	}

	$effect(() => {
		if (!triggerRef) {
			interactiveElement = null;
			return;
		}

		interactiveElement = findInteractiveElement();

		mutationObserver?.disconnect();
		mutationObserver = new MutationObserver(() => {
			interactiveElement = findInteractiveElement();
		});
		mutationObserver.observe(triggerRef, { childList: true, subtree: true });

		return () => {
			mutationObserver?.disconnect();
			mutationObserver = null;
		};
	});

	$effect(() => {
		ctx.setTriggerElement(interactiveElement);
	});

	$effect(() => {
		const target = interactiveElement;
		const expanded = ctx.isOpen;
		const isDisabled = disabled;

		if (target !== managedElement) {
			cleanupManaged(managedElement);
			managedElement = target;
		}

		applyAttributes(managedElement);

		if (managedElement) {
			managedElement.setAttribute('aria-expanded', expanded ? 'true' : 'false');
			if (isDisabled) {
				managedElement.setAttribute('aria-disabled', 'true');
				if (!isNativeInteractive(managedElement)) {
					managedElement.tabIndex = -1;
				}
			} else {
				managedElement.removeAttribute('aria-disabled');
				if (!isNativeInteractive(managedElement)) {
					managedElement.tabIndex = 0;
				}
			}
		}
	});
</script>

<div
	bind:this={triggerRef}
	class="gr-menu-trigger {className}"
	aria-disabled={disabled || undefined}
	onclick={handleClick}
	onkeydown={handleKeyDown}
	{...restProps}
>
	{@render children()}
</div>

<style>
	.gr-menu-trigger {
		display: inline-flex;
		align-items: center;
		cursor: pointer;
	}

	.gr-menu-trigger[aria-disabled='true'] {
		cursor: not-allowed;
		opacity: 0.5;
	}
</style>
