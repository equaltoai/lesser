import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const BarChart: import('svelte').Component<Props, object, ''>;
type BarChart = ReturnType<typeof BarChart>;
export default BarChart;
//# sourceMappingURL=bar-chart.svelte.d.ts.map
