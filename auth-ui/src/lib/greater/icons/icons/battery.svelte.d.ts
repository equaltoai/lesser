import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Battery: import('svelte').Component<Props, object, ''>;
type Battery = ReturnType<typeof Battery>;
export default Battery;
//# sourceMappingURL=battery.svelte.d.ts.map
