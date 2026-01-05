interface Props {
	checked?: boolean;
	disabled?: boolean;
	required?: boolean;
	indeterminate?: boolean;
	class?: string;
	id?: string;
	name?: string;
	value?: string;
	onchange?: (checked: boolean) => void;
}
declare const Checkbox: import('svelte').Component<Props, {}, 'checked'>;
type Checkbox = ReturnType<typeof Checkbox>;
export default Checkbox;
//# sourceMappingURL=Checkbox.svelte.d.ts.map
