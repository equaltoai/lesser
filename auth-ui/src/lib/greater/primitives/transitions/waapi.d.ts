import type { TransitionConfig } from 'svelte/transition';
export interface WaapiTransitionConfig {
	delay: number;
	duration: number;
	easing: (t: number) => number;
}
export declare function waapiTransition(
	node: Element,
	keyframes: Keyframe[],
	config: WaapiTransitionConfig
): TransitionConfig;
//# sourceMappingURL=waapi.d.ts.map
