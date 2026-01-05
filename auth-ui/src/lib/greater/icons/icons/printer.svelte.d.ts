import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Printer: import('svelte').Component<Props, object, ''>;
type Printer = ReturnType<typeof Printer>;
export default Printer;
//# sourceMappingURL=printer.svelte.d.ts.map
