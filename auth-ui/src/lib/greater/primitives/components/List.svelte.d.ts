import type { HTMLAttributes } from 'svelte/elements';
import { type Component, type Snippet } from 'svelte';
type MaxWidthPreset = 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full';
interface Props extends HTMLAttributes<HTMLUListElement | HTMLOListElement> {
	icon?: Component;
	iconColor?: 'primary' | 'success' | 'warning' | 'error' | 'gray';
	iconSize?: number;
	spacing?: 'sm' | 'md' | 'lg';
	maxWidth?: MaxWidthPreset;
	ordered?: boolean;
	class?: string;
	children: Snippet;
}
declare const List: Component<Props, {}, ''>;
type List = ReturnType<typeof List>;
export default List;
//# sourceMappingURL=List.svelte.d.ts.map
