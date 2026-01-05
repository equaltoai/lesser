import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Globe: import('svelte').Component<Props, object, ''>;
type Globe = ReturnType<typeof Globe>;
export default Globe;
//# sourceMappingURL=globe.svelte.d.ts.map
