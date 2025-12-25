import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Camera: import('svelte').Component<Props, object, ''>;
type Camera = ReturnType<typeof Camera>;
export default Camera;
//# sourceMappingURL=camera.svelte.d.ts.map
