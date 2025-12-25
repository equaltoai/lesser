import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Settings: import('svelte').Component<Props, object, ''>;
type Settings = ReturnType<typeof Settings>;
export default Settings;
//# sourceMappingURL=settings.svelte.d.ts.map
