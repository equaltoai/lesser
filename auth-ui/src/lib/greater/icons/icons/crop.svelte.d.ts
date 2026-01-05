import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Crop: import('svelte').Component<Props, object, ''>;
type Crop = ReturnType<typeof Crop>;
export default Crop;
//# sourceMappingURL=crop.svelte.d.ts.map
