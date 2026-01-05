/**
 * @equaltoai/greater-components-headless
 *
 * Headless UI primitives providing behavior without styling.
 *
 * All primitives follow the same pattern:
 * - Pure behavior, zero styling
 * - Svelte actions for DOM integration
 * - Full TypeScript support
 * - WCAG 2.1 AA compliant
 * - Tree-shakeable exports
 */

// Button primitive
export { createButton, isButton } from './primitives/button';
export type { ButtonState, ButtonConfig, ButtonBuilder } from './primitives/button';

// Modal primitive
export { createModal } from './primitives/modal';
export type { ModalState, ModalConfig, ModalActions, ModalHelpers } from './primitives/modal';

// Menu primitive
export { createMenu } from './primitives/menu';
export type { MenuState, MenuConfig, MenuActions, MenuHelpers } from './primitives/menu';

// Tooltip primitive
export { createTooltip } from './primitives/tooltip';
export type {
	TooltipState,
	TooltipConfig,
	TooltipActions,
	TooltipHelpers,
	TooltipPlacement,
} from './primitives/tooltip';

// Tabs primitive
export { createTabs } from './primitives/tabs';
export type {
	TabsState,
	TabsConfig,
	TabsActions,
	TabsHelpers,
	TabsOrientation,
	TabItem,
	PanelItem,
	TabActionOptions,
	PanelActionOptions,
} from './primitives/tabs';

// TextField primitive
export { createTextField } from './primitives/textfield';
export type {
	TextFieldState,
	TextFieldConfig,
	TextFieldActions,
	TextFieldHelpers,
	TextField,
} from './primitives/textfield';

// Avatar primitive
export { createAvatar } from './primitives/avatar';
export type {
	AvatarState,
	AvatarConfig,
	AvatarActions,
	AvatarHelpers,
	Avatar,
} from './primitives/avatar';

// Skeleton primitive
export { createSkeleton } from './primitives/skeleton';
export type {
	SkeletonState,
	SkeletonConfig,
	SkeletonActions,
	SkeletonHelpers,
	Skeleton,
} from './primitives/skeleton';

// Spinner primitive
export { createSpinner } from './primitives/spinner';
export type {
	SpinnerState,
	SpinnerConfig,
	SpinnerActions,
	SpinnerHelpers,
	Spinner,
	SpinnerSize,
	SpinnerColor,
} from './primitives/spinner';

// Alert primitive
export { createAlert, isAlertVariant } from './primitives/alert';
export type { AlertVariant, AlertState, AlertConfig, AlertBuilder } from './primitives/alert';

// Common types
export type { Action, ActionReturn } from './types/common';

// Utilities
export { generateId } from './utils/id';
export {
	isActivationKey,
	isEscapeKey,
	isArrowKey,
	isTabKey,
	getArrowKeyDirection,
	getNavigationDirection,
} from './utils/keyboard';
export type { NavigationDirection } from './utils/keyboard';

// Behaviors - Framework-agnostic utilities
export {
	// Focus Trap
	createFocusTrap,
	getFocusableElements,
	getFirstFocusable,
	getLastFocusable,
	isFocusable,
	// Roving Tabindex
	createRovingTabindex,
	// Typeahead
	createTypeahead,
	// Popover Positioning
	createPopover,
	// Dismissable
	createDismissable,
	getDismissableLayerCount,
	clearDismissableLayers,
	// Live Region
	createLiveRegion,
	getGlobalLiveRegion,
	announce,
	announcePolite,
	announceAssertive,
} from './behaviors';

export type {
	// Focus Trap types
	FocusTrapConfig,
	FocusTrapState,
	FocusTrap,
	// Roving Tabindex types
	RovingOrientation,
	RovingTabindexConfig,
	RovingTabindexState,
	RovingTabindex,
	// Typeahead types
	TypeaheadMatchMode,
	TypeaheadConfig,
	TypeaheadState,
	Typeahead,
	// Popover types
	PopoverPlacement,
	PopoverAlignment,
	PopoverSide,
	PopoverConfig,
	PopoverPosition,
	PopoverState,
	Popover,
	// Dismissable types
	DismissableConfig,
	DismissReason,
	DismissableState,
	Dismissable,
	// Live Region types
	LiveRegionPoliteness,
	LiveRegionConfig,
	AnnounceOptions,
	LiveRegionState,
	LiveRegion,
} from './behaviors';
