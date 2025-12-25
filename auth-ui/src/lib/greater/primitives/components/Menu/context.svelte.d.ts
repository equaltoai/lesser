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
	x: number;
	y: number;
	placement: MenuPlacement;
}
export type MenuContextValue = MenuState;
export declare class MenuState {
	isOpen: boolean;
	activeIndex: number;
	items: MenuItemConfig[];
	position: MenuPosition;
	triggerElement: HTMLElement | null;
	contentElement: HTMLElement | null;
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
	});
	open: () => void;
	close: () => void;
	toggle: () => void;
	setActiveIndex: (index: number) => void;
	registerItem: (item: MenuItemConfig) => void;
	unregisterItem: (id: string) => void;
	selectItem: (id: string) => void;
	setTriggerElement: (el: HTMLElement | null) => void;
	setContentElement: (el: HTMLElement | null) => void;
	updatePosition: () => void;
}
export declare function createMenuContext(state: MenuState): void;
export declare function getMenuContext(): MenuState;
export declare function generateMenuId(): string;
//# sourceMappingURL=context.svelte.d.ts.map
