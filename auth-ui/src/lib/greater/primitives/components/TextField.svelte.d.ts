import type { HTMLInputAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
interface Props extends Omit<HTMLInputAttributes, 'type' | 'value'> {
	label?: string;
	value?: string;
	type?: 'text' | 'email' | 'password' | 'url' | 'tel' | 'search';
	placeholder?: string;
	invalid?: boolean;
	disabled?: boolean;
	readonly?: boolean;
	required?: boolean;
	helpText?: string;
	errorMessage?: string;
	class?: string;
	inputClass?: string;
	prefix?: Snippet;
	suffix?: Snippet;
}
declare const TextField: import('svelte').Component<Props, {}, 'value'>;
type TextField = ReturnType<typeof TextField>;
export default TextField;
//# sourceMappingURL=TextField.svelte.d.ts.map
