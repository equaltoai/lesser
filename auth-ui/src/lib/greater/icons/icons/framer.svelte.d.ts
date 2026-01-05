import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Framer: import('svelte').Component<Props, object, ''>;
type Framer = ReturnType<typeof Framer>;
export default Framer;
//# sourceMappingURL=framer.svelte.d.ts.map
