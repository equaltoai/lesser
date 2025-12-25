import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Mention: import('svelte').Component<Props, object, ''>;
type Mention = ReturnType<typeof Mention>;
export default Mention;
//# sourceMappingURL=mention.svelte.d.ts.map
