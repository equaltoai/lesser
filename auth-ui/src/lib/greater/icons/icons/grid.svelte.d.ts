import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Grid: import('svelte').Component<Props, object, ''>;
type Grid = ReturnType<typeof Grid>;
export default Grid;
//# sourceMappingURL=grid.svelte.d.ts.map
