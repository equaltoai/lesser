import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const MapPin: import('svelte').Component<Props, object, ''>;
type MapPin = ReturnType<typeof MapPin>;
export default MapPin;
//# sourceMappingURL=map-pin.svelte.d.ts.map
