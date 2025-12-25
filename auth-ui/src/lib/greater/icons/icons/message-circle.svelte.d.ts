import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const MessageCircle: import('svelte').Component<Props, object, ''>;
type MessageCircle = ReturnType<typeof MessageCircle>;
export default MessageCircle;
//# sourceMappingURL=message-circle.svelte.d.ts.map
