type CspSafeKeyframe = Keyframe;

const elementStyles = new WeakMap<Element, Animation>();

export interface ApplyCspSafeStylesOptions extends KeyframeAnimationOptions {
	/**
	 * A tiny non-zero duration helps ensure the UA applies the final keyframe
	 * even when we immediately pause the animation at the end.
	 *
	 * @default 1
	 */
	duration?: number;
}

function hasAnimate(element: Element): element is Element & { animate: Element['animate'] } {
	return typeof (element as Element).animate === 'function';
}

export function applyCspSafeStyles(
	element: Element,
	styles: CspSafeKeyframe,
	options: ApplyCspSafeStylesOptions = {}
): Animation | null {
	if (!hasAnimate(element)) return null;

	const duration = options.duration ?? 1;
	const animation = elementStyles.get(element);

	if (animation) {
		const effect = animation.effect;
		if (effect && 'setKeyframes' in effect) {
			(effect as KeyframeEffect).setKeyframes([styles]);
		} else {
			animation.cancel();
			elementStyles.delete(element);
			return applyCspSafeStyles(element, styles, options);
		}

		try {
			animation.currentTime = duration;
			animation.pause();
		} catch {
			// Ignore: environments without a full WAAPI implementation (e.g. test DOMs)
		}

		return animation;
	}

	const next = element.animate([styles], {
		...options,
		duration,
		fill: options.fill ?? 'forwards',
		easing: options.easing ?? 'linear',
	});

	try {
		next.currentTime = duration;
		next.pause();
	} catch {
		// Ignore: environments without a full WAAPI implementation (e.g. test DOMs)
	}

	elementStyles.set(element, next);
	return next;
}

export function clearCspSafeStyles(element: Element): void {
	const animation = elementStyles.get(element);
	if (!animation) return;
	animation.cancel();
	elementStyles.delete(element);
}
