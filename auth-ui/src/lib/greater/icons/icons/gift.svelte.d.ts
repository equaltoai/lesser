import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Gift: import('svelte').Component<Props, object, ''>;
type Gift = ReturnType<typeof Gift>;
export default Gift;
//# sourceMappingURL=gift.svelte.d.ts.map
