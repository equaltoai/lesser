import { type Component, type Snippet } from 'svelte';
interface Props {
	icon?: Component;
	iconColor?: 'primary' | 'success' | 'warning' | 'error' | 'gray';
	iconSize?: number;
	spacing?: 'sm' | 'md' | 'lg';
	maxWidth?: string | number;
	ordered?: boolean;
	class?: string;
	children: Snippet;
}
declare const List: Component<Props, {}, ''>;
type List = ReturnType<typeof List>;
export default List;
//# sourceMappingURL=List.svelte.d.ts.map
