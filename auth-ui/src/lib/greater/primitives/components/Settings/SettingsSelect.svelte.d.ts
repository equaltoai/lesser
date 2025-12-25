interface Props<T = string> {
	label: string;
	description?: string;
	value: T;
	options: Array<{
		value: T;
		label: string;
	}>;
	disabled?: boolean;
}
declare const SettingsSelect: import('svelte').Component<Props<string>, {}, 'value'>;
type SettingsSelect = ReturnType<typeof SettingsSelect>;
export default SettingsSelect;
//# sourceMappingURL=SettingsSelect.svelte.d.ts.map
