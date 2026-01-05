import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Link: import('svelte').Component<Props, object, ''>;
type Link = ReturnType<typeof Link>;
export default Link;
//# sourceMappingURL=link.svelte.d.ts.map
