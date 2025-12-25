import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Clipboard: import('svelte').Component<Props, object, ''>;
type Clipboard = ReturnType<typeof Clipboard>;
export default Clipboard;
//# sourceMappingURL=clipboard.svelte.d.ts.map
