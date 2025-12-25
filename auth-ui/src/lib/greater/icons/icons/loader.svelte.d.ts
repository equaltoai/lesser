import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Loader: import('svelte').Component<Props, object, ''>;
type Loader = ReturnType<typeof Loader>;
export default Loader;
//# sourceMappingURL=loader.svelte.d.ts.map
