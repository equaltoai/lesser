import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Octagon: import('svelte').Component<Props, object, ''>;
type Octagon = ReturnType<typeof Octagon>;
export default Octagon;
//# sourceMappingURL=octagon.svelte.d.ts.map
