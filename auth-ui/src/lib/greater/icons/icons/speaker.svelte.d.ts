import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Speaker: import('svelte').Component<Props, object, ''>;
type Speaker = ReturnType<typeof Speaker>;
export default Speaker;
//# sourceMappingURL=speaker.svelte.d.ts.map
