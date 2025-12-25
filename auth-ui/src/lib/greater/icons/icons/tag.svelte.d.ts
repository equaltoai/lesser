import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Tag: import('svelte').Component<Props, object, ''>;
type Tag = ReturnType<typeof Tag>;
export default Tag;
//# sourceMappingURL=tag.svelte.d.ts.map
