import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Upload: import('svelte').Component<Props, object, ''>;
type Upload = ReturnType<typeof Upload>;
export default Upload;
//# sourceMappingURL=upload.svelte.d.ts.map
