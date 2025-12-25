import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Layout: import('svelte').Component<Props, object, ''>;
type Layout = ReturnType<typeof Layout>;
export default Layout;
//# sourceMappingURL=layout.svelte.d.ts.map
