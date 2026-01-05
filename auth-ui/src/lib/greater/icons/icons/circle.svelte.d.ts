import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Circle: import('svelte').Component<Props, object, ''>;
type Circle = ReturnType<typeof Circle>;
export default Circle;
//# sourceMappingURL=circle.svelte.d.ts.map
