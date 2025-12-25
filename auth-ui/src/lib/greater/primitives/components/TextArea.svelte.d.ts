import type { HTMLTextAreaAttributes } from 'svelte/elements';
interface Props extends Omit<HTMLTextAreaAttributes, 'value'> {
	value?: string;
	label?: string;
	helpText?: string;
	errorMessage?: string;
	textareaClass?: string;
	class?: string;
	invalid?: boolean;
	onchange?: (value: string) => void;
	oninput?: (value: string) => void;
}
declare const TextArea: import('svelte').Component<Props, {}, 'value'>;
type TextArea = ReturnType<typeof TextArea>;
export default TextArea;
//# sourceMappingURL=TextArea.svelte.d.ts.map
