import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Command: import('svelte').Component<Props, object, ''>;
type Command = ReturnType<typeof Command>;
export default Command;
//# sourceMappingURL=command.svelte.d.ts.map
