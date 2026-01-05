import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const ZoomOut: import('svelte').Component<Props, object, ''>;
type ZoomOut = ReturnType<typeof ZoomOut>;
export default ZoomOut;
//# sourceMappingURL=zoom-out.svelte.d.ts.map
