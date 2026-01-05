export interface SelectOption {
	value: string;
	label: string;
	disabled?: boolean;
}
interface Props {
	value?: string;
	options: SelectOption[];
	placeholder?: string;
	disabled?: boolean;
	required?: boolean;
	class?: string;
	id?: string;
	name?: string;
	onchange?: (value: string) => void;
}
declare const Select: import('svelte').Component<Props, {}, 'value'>;
type Select = ReturnType<typeof Select>;
export default Select;
//# sourceMappingURL=Select.svelte.d.ts.map
