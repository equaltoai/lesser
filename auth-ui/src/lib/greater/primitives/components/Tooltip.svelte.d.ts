import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
	content: string;
	placement?: 'top' | 'bottom' | 'left' | 'right' | 'auto';
	trigger?: 'hover' | 'focus' | 'click' | 'manual';
	delay?:
		| {
				show?: number;
				hide?: number;
		  }
		| number;
	disabled?: boolean;
	class?: string;
	children: Snippet;
}
declare const Tooltip: import('svelte').Component<Props, {}, ''>;
type Tooltip = ReturnType<typeof Tooltip>;
export default Tooltip;
//# sourceMappingURL=Tooltip.svelte.d.ts.map
