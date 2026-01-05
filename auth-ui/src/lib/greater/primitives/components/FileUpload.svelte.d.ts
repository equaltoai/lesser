interface Props {
	accept?: string;
	multiple?: boolean;
	disabled?: boolean;
	class?: string;
	id?: string;
	name?: string;
	maxSize?: number;
	onchange?: (files: FileList | null) => void;
}
declare const FileUpload: import('svelte').Component<Props, {}, ''>;
type FileUpload = ReturnType<typeof FileUpload>;
export default FileUpload;
//# sourceMappingURL=FileUpload.svelte.d.ts.map
