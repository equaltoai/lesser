import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Box: import('svelte').Component<Props, object, ''>;
type Box = ReturnType<typeof Box>;
export default Box;
//# sourceMappingURL=box.svelte.d.ts.map
