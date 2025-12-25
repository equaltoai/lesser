import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Book: import('svelte').Component<Props, object, ''>;
type Book = ReturnType<typeof Book>;
export default Book;
//# sourceMappingURL=book.svelte.d.ts.map
