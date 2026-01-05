/**
 * Headless Behaviors
 *
 * Framework-agnostic utilities for common UI behaviors.
 * These utilities provide accessibility and interaction patterns
 * without any visual styling.
 *
 * @module @equaltoai/greater-components-headless/behaviors
 */

// Focus Trap
export {
	createFocusTrap,
	getFocusableElements,
	getFirstFocusable,
	getLastFocusable,
	isFocusable,
} from './focus-trap';
export type { FocusTrapConfig, FocusTrapState, FocusTrap } from './focus-trap';

// Roving Tabindex
export { createRovingTabindex } from './roving-tabindex';
export type {
	RovingOrientation,
	RovingTabindexConfig,
	RovingTabindexState,
	RovingTabindex,
} from './roving-tabindex';

// Typeahead
export { createTypeahead } from './typeahead';
export type { TypeaheadMatchMode, TypeaheadConfig, TypeaheadState, Typeahead } from './typeahead';

// Popover Positioning
export { createPopover } from './popover';
export type {
	PopoverPlacement,
	PopoverAlignment,
	PopoverSide,
	PopoverConfig,
	PopoverPosition,
	PopoverState,
	Popover,
} from './popover';

// Dismissable
export { createDismissable, getDismissableLayerCount, clearDismissableLayers } from './dismissable';
export type {
	DismissableConfig,
	DismissReason,
	DismissableState,
	Dismissable,
} from './dismissable';

// Live Region
export {
	createLiveRegion,
	getGlobalLiveRegion,
	announce,
	announcePolite,
	announceAssertive,
} from './live-region';
export type {
	LiveRegionPoliteness,
	LiveRegionConfig,
	AnnounceOptions,
	LiveRegionState,
	LiveRegion,
} from './live-region';
