import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const ZoomIn: import('svelte').Component<Props, object, ''>;
type ZoomIn = ReturnType<typeof ZoomIn>;
export default ZoomIn;
//# sourceMappingURL=zoom-in.svelte.d.ts.map
