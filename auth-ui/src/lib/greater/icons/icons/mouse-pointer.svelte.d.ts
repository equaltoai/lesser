import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const MousePointer: import('svelte').Component<Props, object, ''>;
type MousePointer = ReturnType<typeof MousePointer>;
export default MousePointer;
//# sourceMappingURL=mouse-pointer.svelte.d.ts.map
