import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Sidebar: import('svelte').Component<Props, object, ''>;
type Sidebar = ReturnType<typeof Sidebar>;
export default Sidebar;
//# sourceMappingURL=sidebar.svelte.d.ts.map
