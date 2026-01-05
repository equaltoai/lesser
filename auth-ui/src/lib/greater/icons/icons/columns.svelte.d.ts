import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Columns: import('svelte').Component<Props, object, ''>;
type Columns = ReturnType<typeof Columns>;
export default Columns;
//# sourceMappingURL=columns.svelte.d.ts.map
