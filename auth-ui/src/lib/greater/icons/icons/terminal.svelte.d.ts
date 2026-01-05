import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Terminal: import('svelte').Component<Props, object, ''>;
type Terminal = ReturnType<typeof Terminal>;
export default Terminal;
//# sourceMappingURL=terminal.svelte.d.ts.map
