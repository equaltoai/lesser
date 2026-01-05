import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
interface Props extends HTMLAttributes<HTMLDivElement> {
	variant?: 'text' | 'circular' | 'rectangular' | 'rounded';
	width?: string | number;
	height?: string | number;
	animation?: 'pulse' | 'wave' | 'none';
	class?: string;
	loading?: boolean;
	children?: Snippet;
}
declare const Skeleton: import('svelte').Component<Props, {}, ''>;
type Skeleton = ReturnType<typeof Skeleton>;
export default Skeleton;
//# sourceMappingURL=Skeleton.svelte.d.ts.map
