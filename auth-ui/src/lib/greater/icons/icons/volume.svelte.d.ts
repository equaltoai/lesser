import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Volume: import('svelte').Component<Props, object, ''>;
type Volume = ReturnType<typeof Volume>;
export default Volume;
//# sourceMappingURL=volume.svelte.d.ts.map
