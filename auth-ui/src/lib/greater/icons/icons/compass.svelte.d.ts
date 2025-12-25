import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Compass: import('svelte').Component<Props, object, ''>;
type Compass = ReturnType<typeof Compass>;
export default Compass;
//# sourceMappingURL=compass.svelte.d.ts.map
