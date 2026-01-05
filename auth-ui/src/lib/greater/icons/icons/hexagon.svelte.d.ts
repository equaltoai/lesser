import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Hexagon: import('svelte').Component<Props, object, ''>;
type Hexagon = ReturnType<typeof Hexagon>;
export default Hexagon;
//# sourceMappingURL=hexagon.svelte.d.ts.map
