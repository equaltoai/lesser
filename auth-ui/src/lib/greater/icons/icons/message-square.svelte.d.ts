import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const MessageSquare: import('svelte').Component<Props, object, ''>;
type MessageSquare = ReturnType<typeof MessageSquare>;
export default MessageSquare;
//# sourceMappingURL=message-square.svelte.d.ts.map
