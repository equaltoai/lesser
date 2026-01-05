import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const PenTool: import('svelte').Component<Props, object, ''>;
type PenTool = ReturnType<typeof PenTool>;
export default PenTool;
//# sourceMappingURL=pen-tool.svelte.d.ts.map
