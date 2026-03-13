import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
export type Placement = 'top' | 'bottom' | 'left' | 'right' | 'auto';
interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
	content: string;
	id?: string;
	placement?: Placement;
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
