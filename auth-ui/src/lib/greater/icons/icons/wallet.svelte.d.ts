import type { SVGAttributes } from 'svelte/elements';
interface Props extends SVGAttributes<SVGSVGElement> {
	size?: number | string;
	color?: string;
	strokeWidth?: number | string;
	class?: string;
}
declare const Wallet: import('svelte').Component<Props, object, ''>;
type Wallet = ReturnType<typeof Wallet>;
export default Wallet;
//# sourceMappingURL=wallet.svelte.d.ts.map
