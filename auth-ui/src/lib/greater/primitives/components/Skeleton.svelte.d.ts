import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
export type WidthPreset = 'full' | '1/2' | '1/3' | '2/3' | '1/4' | '3/4' | 'content' | 'auto';
export type HeightPreset = 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
interface Props extends HTMLAttributes<HTMLDivElement> {
	variant?: 'text' | 'circular' | 'rectangular' | 'rounded';
	width?: WidthPreset;
	height?: HeightPreset;
	animation?: 'pulse' | 'wave' | 'none';
	class?: string;
	loading?: boolean;
	children?: Snippet;
}
declare const Skeleton: import('svelte').Component<Props, {}, ''>;
type Skeleton = ReturnType<typeof Skeleton>;
export default Skeleton;
//# sourceMappingURL=Skeleton.svelte.d.ts.map
