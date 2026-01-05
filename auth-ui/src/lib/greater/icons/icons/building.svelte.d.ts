import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Building: import('svelte').Component<Props, object, ''>;
type Building = ReturnType<typeof Building>;
export default Building;
//# sourceMappingURL=building.svelte.d.ts.map
