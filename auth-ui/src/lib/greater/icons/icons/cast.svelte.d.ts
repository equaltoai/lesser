import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Cast: import('svelte').Component<Props, object, ''>;
type Cast = ReturnType<typeof Cast>;
export default Cast;
//# sourceMappingURL=cast.svelte.d.ts.map
