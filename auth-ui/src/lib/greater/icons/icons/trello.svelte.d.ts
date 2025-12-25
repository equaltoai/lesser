import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Trello: import('svelte').Component<Props, object, ''>;
type Trello = ReturnType<typeof Trello>;
export default Trello;
//# sourceMappingURL=trello.svelte.d.ts.map
