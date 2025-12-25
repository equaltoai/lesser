import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const PieChart: import('svelte').Component<Props, object, ''>;
type PieChart = ReturnType<typeof PieChart>;
export default PieChart;
//# sourceMappingURL=pie-chart.svelte.d.ts.map
