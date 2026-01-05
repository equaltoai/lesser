import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Square: import('svelte').Component<Props, object, ''>;
type Square = ReturnType<typeof Square>;
export default Square;
//# sourceMappingURL=square.svelte.d.ts.map
