import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Clock: import('svelte').Component<Props, object, ''>;
type Clock = ReturnType<typeof Clock>;
export default Clock;
//# sourceMappingURL=clock.svelte.d.ts.map
