import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Lock: import('svelte').Component<Props, object, ''>;
type Lock = ReturnType<typeof Lock>;
export default Lock;
//# sourceMappingURL=lock.svelte.d.ts.map
