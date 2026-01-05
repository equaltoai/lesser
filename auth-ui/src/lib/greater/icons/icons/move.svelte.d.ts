import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Move: import('svelte').Component<Props, object, ''>;
type Move = ReturnType<typeof Move>;
export default Move;
//# sourceMappingURL=move.svelte.d.ts.map
