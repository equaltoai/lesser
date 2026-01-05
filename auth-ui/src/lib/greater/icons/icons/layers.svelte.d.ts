import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Layers: import('svelte').Component<Props, object, ''>;
type Layers = ReturnType<typeof Layers>;
export default Layers;
//# sourceMappingURL=layers.svelte.d.ts.map
