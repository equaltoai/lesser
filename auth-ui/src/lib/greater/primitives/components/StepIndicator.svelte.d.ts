import type { Component } from 'svelte';
interface Props {
	/**
	 * Step number or character.
	 */
	number: number | string;
	/**
	 * Optional label displayed below the indicator.
	 */
	label?: string;
	/**
	 * Visual style variant.
	 */
	variant?: 'filled' | 'outlined' | 'ghost';
	/**
	 * Size of the indicator.
	 */
	size?: 'sm' | 'md' | 'lg';
	/**
	 * Current state of the step.
	 */
	state?: 'pending' | 'active' | 'completed' | 'error';
	/**
	 * Color theme (overridden by state in some cases).
	 */
	color?: 'primary' | 'success' | 'warning' | 'error';
	/**
	 * Custom icon to display instead of number.
	 */
	icon?: Component;
	/**
	 * Additional CSS classes.
	 */
	class?: string;
}
/**
 * StepIndicator component - Numbered badge for tutorials and multi-step workflows.
 *
 *
 * @example
 * ```svelte
 * <StepIndicator number={1} state="active" label="Step 1" />
 * <StepIndicator number={2} state="pending" />
 * <StepIndicator number={3} state="completed" />
 * ```
 */
declare const StepIndicator: Component<Props, {}, ''>;
type StepIndicator = ReturnType<typeof StepIndicator>;
export default StepIndicator;
//# sourceMappingURL=StepIndicator.svelte.d.ts.map
