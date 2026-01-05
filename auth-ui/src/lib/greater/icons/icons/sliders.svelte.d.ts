import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Sliders: import('svelte').Component<Props, object, ''>;
type Sliders = ReturnType<typeof Sliders>;
export default Sliders;
//# sourceMappingURL=sliders.svelte.d.ts.map
