import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Crosshair: import('svelte').Component<Props, object, ''>;
type Crosshair = ReturnType<typeof Crosshair>;
export default Crosshair;
//# sourceMappingURL=crosshair.svelte.d.ts.map
