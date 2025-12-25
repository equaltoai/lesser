import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
interface TabData {
	id: string;
	label: string;
	disabled?: boolean;
	content?: Snippet;
}
interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'role'> {
	tabs: TabData[];
	activeTab?: string;
	orientation?: 'horizontal' | 'vertical';
	activation?: 'automatic' | 'manual';
	variant?: 'default' | 'pills' | 'underline';
	class?: string;
	onTabChange?: (tabId: string) => void;
}
declare const Tabs: import('svelte').Component<Props, {}, ''>;
type Tabs = ReturnType<typeof Tabs>;
export default Tabs;
//# sourceMappingURL=Tabs.svelte.d.ts.map
