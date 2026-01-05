import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Server: import('svelte').Component<Props, object, ''>;
type Server = ReturnType<typeof Server>;
export default Server;
//# sourceMappingURL=server.svelte.d.ts.map
