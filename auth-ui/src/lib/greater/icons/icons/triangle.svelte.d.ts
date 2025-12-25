import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Triangle: import('svelte').Component<Props, object, ''>;
type Triangle = ReturnType<typeof Triangle>;
export default Triangle;
//# sourceMappingURL=triangle.svelte.d.ts.map
