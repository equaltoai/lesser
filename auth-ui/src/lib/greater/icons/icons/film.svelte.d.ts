import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Film: import('svelte').Component<Props, object, ''>;
type Film = ReturnType<typeof Film>;
export default Film;
//# sourceMappingURL=film.svelte.d.ts.map
