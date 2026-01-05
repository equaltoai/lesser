import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Slack: import('svelte').Component<Props, object, ''>;
type Slack = ReturnType<typeof Slack>;
export default Slack;
//# sourceMappingURL=slack.svelte.d.ts.map
