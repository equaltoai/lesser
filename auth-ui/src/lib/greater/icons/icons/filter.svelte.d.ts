import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Filter: import('svelte').Component<Props, object, ''>;
type Filter = ReturnType<typeof Filter>;
export default Filter;
//# sourceMappingURL=filter.svelte.d.ts.map
