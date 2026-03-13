import { getContext, setContext, tick } from 'svelte';
import { calculatePlacement } from './positioning';

export type MenuPlacement = 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end';

export interface MenuItemConfig {
	id: string;
	label?: string;
	disabled?: boolean;
	destructive?: boolean;
	icon?: unknown;
	shortcut?: string;
	onClick?: () => void;
}

export interface MenuPosition {
	placement: MenuPlacement;
}

const MENU_CONTEXT_KEY = Symbol('menu-context');

export type MenuContextValue = MenuState;

export class MenuState {
	// State
	isOpen = $state(false);
	activeIndex = $state(-1);
	items = $state<MenuItemConfig[]>([]);
	position = $state<MenuPosition>({ placement: 'bottom-start' });
	triggerElement = $state<HTMLElement | null>(null);
	contentElement = $state<HTMLElement | null>(null);

	// Config
	menuId: string;
	triggerId: string;
	placement: MenuPlacement;
	offset: number;
	loop: boolean;
	closeOnSelect: boolean;
	onOpenChange?: (open: boolean) => void;

	constructor(config: {
		menuId: string;
		triggerId: string;
		placement?: MenuPlacement;
		offset?: number;
		loop?: boolean;
		closeOnSelect?: boolean;
		onOpenChange?: (open: boolean) => void;
		initialOpen?: boolean;
	}) {
		this.menuId = config.menuId;
		this.triggerId = config.triggerId;
		this.placement = config.placement ?? 'bottom-start';
		this.offset = config.offset ?? 4;
		this.loop = config.loop ?? true;
		this.closeOnSelect = config.closeOnSelect ?? true;
		this.onOpenChange = config.onOpenChange;

		// Initialize without calling open() to avoid side effects in constructor if possible
		// But we want to respect initialOpen
		this.isOpen = config.initialOpen ?? false;

		this.position = { placement: this.placement };
	}

	open = () => {
		if (this.isOpen) return;
		this.isOpen = true;
		// Reset active index when opening
		this.activeIndex = -1;
		this.onOpenChange?.(true);

		tick().then(() => {
			this.updatePosition();
			// Focus first item
			if (this.items.length > 0) {
				const firstEnabled = this.items.findIndex((item) => !item.disabled);
				if (firstEnabled !== -1) {
					this.activeIndex = firstEnabled;
				}
			}
		});
	};

	close = () => {
		if (!this.isOpen) return;
		this.isOpen = false;
		this.activeIndex = -1;
		this.onOpenChange?.(false);

		tick().then(() => {
			// Restore focus to trigger
			let target = this.triggerElement;

			if (!target || !target.isConnected) {
				target = document.getElementById(this.triggerId);
			}

			if (target) {
				const focusableSelector =
					'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
				const focusable = target.querySelector(focusableSelector) as HTMLElement | null;
				(focusable ?? target)?.focus();
			}
		});
	};

	toggle = () => {
		if (this.isOpen) {
			this.close();
		} else {
			this.open();
		}
	};

	setActiveIndex = (index: number) => {
		if (index >= 0 && index < this.items.length) {
			this.activeIndex = index;
		}
	};

	registerItem = (item: MenuItemConfig) => {
		this.items = [...this.items, item];
	};

	unregisterItem = (id: string) => {
		this.items = this.items.filter((item) => item.id !== id);
	};

	selectItem = (id: string) => {
		const item = this.items.find((i) => i.id === id);
		if (item && !item.disabled) {
			item.onClick?.();
			if (this.closeOnSelect) {
				this.close();
			}
		}
	};

	setTriggerElement = (el: HTMLElement | null) => {
		this.triggerElement = el;
	};

	setContentElement = (el: HTMLElement | null) => {
		this.contentElement = el;
		if (el && this.isOpen) {
			tick().then(() => this.updatePosition());
		}
	};

	updatePosition = () => {
		if (!this.triggerElement || !this.contentElement) return;

		const triggerRect = this.triggerElement.getBoundingClientRect();
		const contentRect = this.contentElement.getBoundingClientRect();

		this.position = {
			placement: calculatePlacement({
				triggerRect,
				contentRect,
				placement: this.placement,
				offset: this.offset,
			}),
		};
	};
}

export function createMenuContext(state: MenuState): void {
	setContext(MENU_CONTEXT_KEY, state);
}

export function getMenuContext(): MenuState {
	const context = getContext<MenuState>(MENU_CONTEXT_KEY);
	if (!context) {
		throw new Error('Menu compound components must be used within a Menu.Root component');
	}
	return context;
}

let menuIdCounter = 0;
export function generateMenuId(): string {
	return `gr-menu-${++menuIdCounter}`;
}
