/**
 * Avatar Primitive
 *
 * Headless primitive for avatar images with fallback, loading states, and status indicators.
 * Provides behavior for displaying user avatars with graceful error handling.
 *
 * @module primitives/avatar
 */

/**
 * Avatar configuration
 */
export interface AvatarConfig {
	/**
	 * Image source URL
	 */
	src?: string;

	/**
	 * Fallback image source
	 */
	fallbackSrc?: string;

	/**
	 * Alt text for image
	 */
	alt?: string;

	/**
	 * Initials to display if image fails to load
	 */
	initials?: string;

	/**
	 * Status indicator
	 */
	status?: 'online' | 'offline' | 'away' | 'busy' | null;

	/**
	 * Whether to show the status indicator
	 */
	showStatus?: boolean;

	/**
	 * Size variant
	 */
	size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';

	/**
	 * Shape variant
	 */
	shape?: 'circle' | 'square' | 'rounded';

	/**
	 * Load handler
	 */
	onLoad?: () => void;

	/**
	 * Error handler
	 */
	onError?: () => void;

	/**
	 * Click handler
	 */
	onClick?: () => void;

	/**
	 * Cleanup handler
	 */
	onDestroy?: () => void;
}

/**
 * Avatar state
 */
export interface AvatarState {
	/**
	 * Current image source
	 */
	currentSrc: string | null;

	/**
	 * Whether the image is loading
	 */
	loading: boolean;

	/**
	 * Whether the image failed to load
	 */
	error: boolean;

	/**
	 * Whether to show fallback content
	 */
	showFallback: boolean;

	/**
	 * Status indicator
	 */
	status: 'online' | 'offline' | 'away' | 'busy' | null;

	/**
	 * Whether to show status indicator
	 */
	showStatus: boolean;

	/**
	 * Size variant
	 */
	size: 'xs' | 'sm' | 'md' | 'lg' | 'xl';

	/**
	 * Shape variant
	 */
	shape: 'circle' | 'square' | 'rounded';

	/**
	 * Alt text
	 */
	alt: string;

	/**
	 * Initials for fallback
	 */
	initials: string;
}

/**
 * Avatar actions
 */
export interface AvatarActions {
	/**
	 * Svelte action for avatar container
	 */
	container: (node: HTMLElement) => { destroy: () => void };

	/**
	 * Svelte action for avatar image
	 */
	image: (node: HTMLImageElement) => { destroy: () => void };
}

/**
 * Avatar helpers
 */
export interface AvatarHelpers {
	/**
	 * Update the image source
	 */
	setSrc: (src: string) => void;

	/**
	 * Update the status
	 */
	setStatus: (status: 'online' | 'offline' | 'away' | 'busy' | null) => void;

	/**
	 * Reload the image
	 */
	reload: () => void;
}

/**
 * Avatar context
 */
export interface Avatar {
	/**
	 * Current state
	 */
	state: AvatarState;

	/**
	 * Actions to apply to DOM elements
	 */
	actions: AvatarActions;

	/**
	 * Helper functions
	 */
	helpers: AvatarHelpers;
}

/**
 * Create an avatar
 *
 * @param config - Avatar configuration
 * @returns Avatar context
 */
export function createAvatar(config: AvatarConfig = {}): Avatar {
	const {
		src,
		fallbackSrc,
		alt = '',
		initials = '',
		status = null,
		showStatus = false,
		size = 'md',
		shape = 'circle',
		onLoad,
		onError,
		onClick,
		onDestroy,
	} = config;

	// Internal state
	let imageElement: HTMLImageElement | null = null;
	let containerElement: HTMLElement | null = null;

	// Reactive state (works in both Svelte and test environments)
	const internalState: AvatarState = {
		currentSrc: src || null,
		loading: !!src,
		error: false,
		showFallback: !src,
		status,
		showStatus,
		size,
		shape,
		alt,
		initials,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof AvatarState, value) {
			target[prop] = value as never;
			updateDOM();
			return true;
		},
	});

	function updateDOM() {
		if (containerElement) {
			if (state.status) {
				containerElement.setAttribute('data-status', state.status);
			} else {
				containerElement.removeAttribute('data-status');
			}
			containerElement.setAttribute('data-size', state.size);
			containerElement.setAttribute('data-shape', state.shape);
		}
		if (imageElement) {
			if (state.currentSrc) {
				imageElement.src = state.currentSrc;
			}
			imageElement.alt = state.alt;
		}
	}

	/**
	 * Handle image load
	 */
	function handleLoad() {
		state.loading = false;
		state.error = false;
		state.showFallback = false;
		onLoad?.();
	}

	/**
	 * Handle image error
	 */
	function handleError() {
		state.loading = false;
		state.error = true;

		// Try fallback if available
		if (fallbackSrc && state.currentSrc !== fallbackSrc) {
			state.currentSrc = fallbackSrc;
			if (imageElement) {
				imageElement.src = fallbackSrc;
			}
		} else {
			// Show fallback content
			state.showFallback = true;
		}

		onError?.();
	}

	/**
	 * Handle click
	 */
	function handleClick() {
		onClick?.();
	}

	/**
	 * Container action
	 */
	function container(node: HTMLElement) {
		containerElement = node;

		// Set role and tabindex if clickable
		if (onClick) {
			node.setAttribute('role', 'button');
			node.setAttribute('tabindex', '0');
			node.addEventListener('click', handleClick);
			node.addEventListener('keydown', (e) => {
				if (e.key === 'Enter' || e.key === ' ') {
					e.preventDefault();
					handleClick();
				}
			});
		}

		// Set data attributes for styling
		node.setAttribute('data-size', state.size);
		node.setAttribute('data-shape', state.shape);
		if (state.status) {
			node.setAttribute('data-status', state.status);
		}

		return {
			destroy() {
				if (onClick && containerElement) {
					node.removeEventListener('click', handleClick);
				}
				containerElement = null;
				onDestroy?.();
			},
		};
	}

	/**
	 * Image action
	 */
	function image(node: HTMLImageElement) {
		imageElement = node;

		// Set initial attributes
		if (state.currentSrc) {
			node.src = state.currentSrc;
		}
		node.alt = state.alt;

		// Add event listeners
		node.addEventListener('load', handleLoad);
		node.addEventListener('error', handleError);

		return {
			destroy() {
				if (imageElement) {
					node.removeEventListener('load', handleLoad);
					node.removeEventListener('error', handleError);
					imageElement = null;
				}
			},
		};
	}

	/**
	 * Set source helper
	 */
	function setSrc(newSrc: string) {
		state.currentSrc = newSrc;
		state.loading = true;
		state.error = false;
		state.showFallback = false;

		if (imageElement) {
			imageElement.src = newSrc;
		}
	}

	/**
	 * Set status helper
	 */
	function setStatus(newStatus: 'online' | 'offline' | 'away' | 'busy' | null) {
		state.status = newStatus;

		if (containerElement) {
			if (newStatus) {
				containerElement.setAttribute('data-status', newStatus);
			} else {
				containerElement.removeAttribute('data-status');
			}
		}
	}

	/**
	 * Reload helper
	 */
	function reload() {
		if (!state.currentSrc) {
			return;
		}

		if (typeof window === 'undefined') {
			// No-op during SSR; the client hydration pass will manage reloads.
			return;
		}

		// Force reload by adding cache-busting parameter
		const url = new URL(state.currentSrc, window.location.href);
		url.searchParams.set('_reload', Date.now().toString());
		setSrc(url.toString());
	}

	return {
		state,
		actions: {
			container,
			image,
		},
		helpers: {
			setSrc,
			setStatus,
			reload,
		},
	};
}
