import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Briefcase: import('svelte').Component<Props, object, ''>;
type Briefcase = ReturnType<typeof Briefcase>;
export default Briefcase;
//# sourceMappingURL=briefcase.svelte.d.ts.map
