import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Image: import('svelte').Component<Props, object, ''>;
type Image = ReturnType<typeof Image>;
export default Image;
//# sourceMappingURL=image.svelte.d.ts.map
