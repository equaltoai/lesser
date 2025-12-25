import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Paperclip: import('svelte').Component<Props, object, ''>;
type Paperclip = ReturnType<typeof Paperclip>;
export default Paperclip;
//# sourceMappingURL=paperclip.svelte.d.ts.map
