import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Calendar: import('svelte').Component<Props, object, ''>;
type Calendar = ReturnType<typeof Calendar>;
export default Calendar;
//# sourceMappingURL=calendar.svelte.d.ts.map
