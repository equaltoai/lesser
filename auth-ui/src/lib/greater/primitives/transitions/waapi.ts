import type { TransitionConfig } from 'svelte/transition';

export interface WaapiTransitionConfig {
	delay: number;
	duration: number;
	easing: (t: number) => number;
}

export function waapiTransition(
	node: Element,
	keyframes: Keyframe[],
	config: WaapiTransitionConfig
): TransitionConfig {
	const { delay, duration, easing } = config;

	if (typeof node.animate !== 'function') {
		return { delay, duration: 0, easing };
	}

	const animation = node.animate(keyframes, {
		duration,
		fill: 'both',
		easing: 'linear',
	});

	try {
		animation.pause();
	} catch {
		// Ignore: environments without a full WAAPI implementation
	}

	return {
		delay,
		duration,
		easing,
		tick: (t: number) => {
			try {
				animation.currentTime = t * duration;
			} catch {
				// Ignore: environments without a full WAAPI implementation
			}
		},
	};
}
