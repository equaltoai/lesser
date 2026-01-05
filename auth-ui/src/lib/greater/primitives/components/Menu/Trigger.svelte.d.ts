import type { Snippet } from 'svelte';
interface Props {
	/** Custom CSS class */
	class?: string;
	/** Trigger content */
	children: Snippet;
	/** Whether trigger is disabled */
	disabled?: boolean;
}
declare const Trigger: import('svelte').Component<Props, {}, ''>;
type Trigger = ReturnType<typeof Trigger>;
export default Trigger;
//# sourceMappingURL=Trigger.svelte.d.ts.map
