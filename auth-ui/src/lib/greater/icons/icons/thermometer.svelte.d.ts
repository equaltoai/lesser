import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Thermometer: import('svelte').Component<Props, object, ''>;
type Thermometer = ReturnType<typeof Thermometer>;
export default Thermometer;
//# sourceMappingURL=thermometer.svelte.d.ts.map
