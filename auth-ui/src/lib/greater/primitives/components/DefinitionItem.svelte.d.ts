import type { HTMLAttributes } from 'svelte/elements';
import { type Component, type Snippet } from 'svelte';
interface Props extends HTMLAttributes<HTMLDivElement> {
	label: string;
	monospace?: boolean;
	wrap?: boolean;
	class?: string;
	children: Snippet;
	actions?: Snippet;
}
declare const DefinitionItem: Component<Props, {}, ''>;
type DefinitionItem = ReturnType<typeof DefinitionItem>;
export default DefinitionItem;
//# sourceMappingURL=DefinitionItem.svelte.d.ts.map

