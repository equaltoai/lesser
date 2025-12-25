import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Droplet: import('svelte').Component<Props, object, ''>;
type Droplet = ReturnType<typeof Droplet>;
export default Droplet;
//# sourceMappingURL=droplet.svelte.d.ts.map
