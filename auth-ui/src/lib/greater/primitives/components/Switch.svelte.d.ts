import type { HTMLInputAttributes } from 'svelte/elements';
interface Props extends Omit<HTMLInputAttributes, 'type' | 'checked' | 'onchange'> {
	checked?: boolean;
	disabled?: boolean;
	class?: string;
	label?: string;
	onchange?: (checked: boolean) => void;
}
declare const Switch: import('svelte').Component<Props, {}, 'checked'>;
type Switch = ReturnType<typeof Switch>;
export default Switch;
//# sourceMappingURL=Switch.svelte.d.ts.map
