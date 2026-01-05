import type { HTMLAttributes } from 'svelte/elements';
import { type Component, type Snippet } from 'svelte';
interface Props extends HTMLAttributes<HTMLDListElement> {
	density?: 'sm' | 'md';
	dividers?: boolean;
	class?: string;
	children: Snippet;
}
declare const DefinitionList: Component<Props, {}, ''>;
type DefinitionList = ReturnType<typeof DefinitionList>;
export default DefinitionList;
//# sourceMappingURL=DefinitionList.svelte.d.ts.map

