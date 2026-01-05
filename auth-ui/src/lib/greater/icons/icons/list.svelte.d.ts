import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const List: import('svelte').Component<Props, object, ''>;
type List = ReturnType<typeof List>;
export default List;
//# sourceMappingURL=list.svelte.d.ts.map
