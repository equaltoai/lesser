/**
 * @fileoverview Greater Primitives - Core UI components for modern web applications
 *
 * This package provides a comprehensive set of primitive UI components built with
 * Svelte 5, following accessibility best practices and design system principles.
 * All components feature full TypeScript support, keyboard navigation, and
 * WCAG 2.1 AA compliance.
 *
 * @version 1.0.0
 * @author Greater Contributors
 * @license AGPL-3.0-only
 * @public
 */
import type { ComponentProps } from 'svelte';
export type { ComponentProps } from 'svelte';
export type {
	PalettePreset,
	CustomPalette,
	ColorScale,
	ColorShade,
	FontPreset,
} from 'src/lib/greater/tokens';
export type { WidthPreset, HeightPreset } from './components/Skeleton.svelte';
export type { ContainerSize, GutterPreset } from './components/Container.svelte';
export type {
	SpacingPreset,
	BackgroundPreset,
	GradientDirection,
} from './components/Section.svelte';
export type { Placement } from './components/Tooltip.svelte';
import Button from './components/Button.svelte';
import CopyButton from './components/CopyButton.svelte';
import TextField from './components/TextField.svelte';
import TextArea from './components/TextArea.svelte';
import Select from './components/Select.svelte';
import Checkbox from './components/Checkbox.svelte';
import Switch from './components/Switch.svelte';
import FileUpload from './components/FileUpload.svelte';
import Modal from './components/Modal.svelte';
import * as Menu from './components/Menu/index';
import Tooltip from './components/Tooltip.svelte';
import Tabs from './components/Tabs.svelte';
import Avatar from './components/Avatar.svelte';
import Skeleton from './components/Skeleton.svelte';
import Card from './components/Card.svelte';
import Container from './components/Container.svelte';
import Heading from './components/Heading.svelte';
import Text from './components/Text.svelte';
import Section from './components/Section.svelte';
import ThemeSwitcher from './components/ThemeSwitcher.svelte';
import ThemeProvider from './components/ThemeProvider.svelte';
import Badge from './components/Badge.svelte';
import List from './components/List.svelte';
import ListItem from './components/ListItem.svelte';
import DefinitionList from './components/DefinitionList.svelte';
import DefinitionItem from './components/DefinitionItem.svelte';
import GradientText from './components/GradientText.svelte';
import StepIndicator from './components/StepIndicator.svelte';
import IconBadge from './components/IconBadge.svelte';
import DropZone from './components/DropZone.svelte';
import StreamingText from './components/StreamingText.svelte';
import Spinner from './components/Spinner.svelte';
import LoadingState from './components/LoadingState.svelte';
import Alert from './components/Alert.svelte';
import SimpleMenu from './components/SimpleMenu.svelte';
/**
 * Core interactive components
 * @public
 */
export {
	/** Accessible button component with loading states and multiple variants. */
	Button,
	/** Button for copying text to clipboard with visual feedback. */
	CopyButton,
	/** Text input field with validation and accessibility features. */
	TextField,
	/** Multi-line text area with auto-resize. */
	TextArea,
	/** Dropdown select component with keyboard navigation. */
	Select,
	/** Checkbox input with indeterminate state support. */
	Checkbox,
	/** Toggle switch component for boolean options. */
	Switch,
	/** File upload component with drag-and-drop support. */
	FileUpload,
	/** Modal dialog with focus management and backdrop handling. */
	Modal,
	/** Dropdown menu compound components. Use Menu.Root, Menu.Trigger, etc. */
	Menu,
	/** Simple menu wrapper for common array-based menu patterns. */
	SimpleMenu,
	/** Tooltip with smart positioning and accessibility. */
	Tooltip,
	/** Tab navigation with keyboard support and ARIA semantics. */
	Tabs,
	/** Avatar component with fallback initials and status indicators. */
	Avatar,
	/** Loading skeleton with animation and shape variants. */
	Skeleton,
	/** Content container with elevation, borders, and semantic sections. */
	Card,
	/** Max-width wrapper for content centering. */
	Container,
	/** Semantic heading with consistent typography. */
	Heading,
	/** Paragraph and inline text component with size, weight, and color variants. */
	Text,
	/** Semantic section wrapper with consistent vertical spacing. */
	Section,
	/** Theme switcher for toggling between color schemes. */
	ThemeSwitcher,
	/** Theme provider for managing application-wide theme state. */
	ThemeProvider,
	/** Badge component for status indicators and labels. */
	Badge,
	/** List component for styled lists with optional icons. */
	List,
	/** ListItem component for items within a List. */
	ListItem,
	/** DefinitionList component for rendering key/value rows. */
	DefinitionList,
	/** DefinitionItem component for label/value pairs within a DefinitionList. */
	DefinitionItem,
	/** GradientText component for eye-catching text effects. */
	GradientText,
	/** StepIndicator component for multi-step workflows. */
	StepIndicator,
	/** IconBadge component for icons in shaped containers. */
	IconBadge,
	/** DropZone component for drag and drop file uploads. */
	DropZone,
	/** StreamingText component for text animation. */
	StreamingText,
	/** Spinner component for loading indicators. */
	Spinner,
	/** LoadingState component for loading states with message and overlay. */
	LoadingState,
	/** Alert component for displaying error, warning, success, and info messages. */
	Alert,
};
/**
 * Settings components for building configuration panels.
 * @public
 */
export { default as SettingsSection } from './components/Settings/SettingsSection.svelte';
export { default as SettingsGroup } from './components/Settings/SettingsGroup.svelte';
export { default as SettingsField } from './components/Settings/SettingsField.svelte';
export { default as SettingsToggle } from './components/Settings/SettingsToggle.svelte';
export { default as SettingsSelect } from './components/Settings/SettingsSelect.svelte';
/**
 * Advanced theme components.
 * @public
 */
export { default as ColorHarmonyPicker } from './components/Theme/ColorHarmonyPicker.svelte';
export { default as ContrastChecker } from './components/Theme/ContrastChecker.svelte';
export { default as ThemeWorkbench } from './components/Theme/ThemeWorkbench.svelte';
export type ButtonProps = ComponentProps<typeof Button>;
export type CopyButtonProps = ComponentProps<typeof CopyButton>;
export type TextFieldProps = ComponentProps<typeof TextField>;
export type TextAreaProps = ComponentProps<typeof TextArea>;
export type SelectProps = ComponentProps<typeof Select>;
export type CheckboxProps = ComponentProps<typeof Checkbox>;
export type SwitchProps = ComponentProps<typeof Switch>;
export type FileUploadProps = ComponentProps<typeof FileUpload>;
export type ModalProps = ComponentProps<typeof Modal>;
export type { MenuPlacement, MenuItemConfig } from './components/Menu/index';
export type { MenuItem as SimpleMenuItem } from './components/SimpleMenu.svelte';
export type TooltipProps = ComponentProps<typeof Tooltip>;
export type TabsProps = ComponentProps<typeof Tabs>;
export type AvatarProps = ComponentProps<typeof Avatar>;
export type SkeletonProps = ComponentProps<typeof Skeleton>;
export type CardProps = ComponentProps<typeof Card>;
export type ContainerProps = ComponentProps<typeof Container>;
export type SectionProps = ComponentProps<typeof Section>;
export type HeadingProps = ComponentProps<typeof Heading>;
export type TextProps = ComponentProps<typeof Text>;
export type ThemeSwitcherProps = ComponentProps<typeof ThemeSwitcher>;
export type ThemeProviderProps = ComponentProps<typeof ThemeProvider>;
export type BadgeProps = ComponentProps<typeof Badge>;
export type ListProps = ComponentProps<typeof List>;
export type ListItemProps = ComponentProps<typeof ListItem>;
export type DefinitionListProps = ComponentProps<typeof DefinitionList>;
export type DefinitionItemProps = ComponentProps<typeof DefinitionItem>;
export type GradientTextProps = ComponentProps<typeof GradientText>;
export type StepIndicatorProps = ComponentProps<typeof StepIndicator>;
export type IconBadgeProps = ComponentProps<typeof IconBadge>;
export type DropZoneProps = ComponentProps<typeof DropZone>;
export type StreamingTextProps = ComponentProps<typeof StreamingText>;
export type SpinnerProps = ComponentProps<typeof Spinner>;
export type LoadingStateProps = ComponentProps<typeof LoadingState>;
export type AlertProps = ComponentProps<typeof Alert>;
export interface SelectOption {
	value: string;
	label: string;
	disabled?: boolean;
}
export { preferencesStore, getPreferences, getPreferenceState } from './stores/preferences';
export type {
	ColorScheme,
	Density,
	FontSize,
	MotionPreference,
	ThemeColors,
	UserPreferences,
	PreferencesState,
} from './stores/preferences';
export {
	fadeUp,
	fadeDown,
	slideIn,
	scaleIn,
	type FadeUpParams,
	type FadeDownParams,
	type SlideInParams,
	type ScaleInParams,
} from './transitions/index.js';
export {
	smoothThemeTransition,
	createSmoothThemeToggle,
	type SmoothThemeTransitionOptions,
} from './utils/index.js';
//# sourceMappingURL=index.d.ts.map
