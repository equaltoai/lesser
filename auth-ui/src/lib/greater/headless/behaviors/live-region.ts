/**
 * Live Region Behavior
 *
 * Framework-agnostic utility for ARIA live announcements.
 * Manages screen reader announcements with proper timing and politeness levels.
 *
 * @module @equaltoai/greater-components-headless/behaviors/live-region
 */

/**
 * Politeness level for announcements
 */
export type LiveRegionPoliteness = 'polite' | 'assertive' | 'off';

/**
 * Live region configuration
 */
export interface LiveRegionConfig {
	/**
	 * Default politeness level
	 * @default 'polite'
	 */
	politeness?: LiveRegionPoliteness;

	/**
	 * Delay between announcements in ms
	 * @default 150
	 */
	announcementDelay?: number;

	/**
	 * Time to keep announcement visible in ms
	 * @default 7000
	 */
	clearDelay?: number;

	/**
	 * Whether to use atomic updates
	 * @default true
	 */
	atomic?: boolean;

	/**
	 * Relevant changes to announce
	 * @default 'additions text'
	 */
	relevant?: string;

	/**
	 * Custom container element (defaults to document.body)
	 */
	container?: HTMLElement;
}

/**
 * Announcement options
 */
export interface AnnounceOptions {
	/**
	 * Politeness level for this announcement
	 */
	politeness?: LiveRegionPoliteness;

	/**
	 * Whether to clear previous announcements
	 * @default false
	 */
	clearPrevious?: boolean;

	/**
	 * Custom delay before this announcement
	 */
	delay?: number;

	/**
	 * Priority (higher priority announcements can interrupt)
	 * @default 0
	 */
	priority?: number;
}

/**
 * Live region state
 */
export interface LiveRegionState {
	/**
	 * Whether live region is initialized
	 */
	initialized: boolean;

	/**
	 * Current announcement queue length
	 */
	queueLength: number;

	/**
	 * Last announcement text
	 */
	lastAnnouncement: string;
}

/**
 * Live region return type
 */
export interface LiveRegion {
	/**
	 * Current state
	 */
	state: LiveRegionState;

	/**
	 * Make an announcement
	 */
	announce: (message: string, options?: AnnounceOptions) => void;

	/**
	 * Make a polite announcement
	 */
	announcePolite: (message: string) => void;

	/**
	 * Make an assertive announcement
	 */
	announceAssertive: (message: string) => void;

	/**
	 * Clear current announcements
	 */
	clear: () => void;

	/**
	 * Update configuration
	 */
	updateConfig: (config: Partial<LiveRegionConfig>) => void;

	/**
	 * Destroy and clean up
	 */
	destroy: () => void;
}

/**
 * Queued announcement
 */
interface QueuedAnnouncement {
	message: string;
	politeness: LiveRegionPoliteness;
	priority: number;
}

/**
 * Create live region elements
 */
function createLiveRegionElement(
	politeness: LiveRegionPoliteness,
	config: Required<LiveRegionConfig>
): HTMLElement {
	const element = document.createElement('div');

	// Visually hidden but accessible to screen readers
	element.style.cssText = `
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	`;

	element.setAttribute('aria-live', politeness);
	element.setAttribute('aria-atomic', String(config.atomic));
	element.setAttribute('aria-relevant', config.relevant);
	element.setAttribute('role', 'status');
	element.setAttribute('data-live-region', politeness);

	return element;
}

/**
 * Create a live region manager
 */
export function createLiveRegion(config: LiveRegionConfig = {}): LiveRegion {
	let currentConfig: Required<LiveRegionConfig> = {
		politeness: config.politeness ?? 'polite',
		announcementDelay: config.announcementDelay ?? 150,
		clearDelay: config.clearDelay ?? 7000,
		atomic: config.atomic ?? true,
		relevant: config.relevant ?? 'additions text',
		container: config.container ?? document.body,
	};

	const state: LiveRegionState = {
		initialized: false,
		queueLength: 0,
		lastAnnouncement: '',
	};

	// Live region elements
	let politeRegion: HTMLElement | null = null;
	let assertiveRegion: HTMLElement | null = null;

	// Announcement queue
	const queue: QueuedAnnouncement[] = [];
	let isProcessing = false;
	let clearTimeoutId: ReturnType<typeof setTimeout> | undefined;

	/**
	 * Initialize live regions
	 */
	function initialize(): void {
		if (state.initialized) return;

		politeRegion = createLiveRegionElement('polite', currentConfig);
		assertiveRegion = createLiveRegionElement('assertive', currentConfig);

		currentConfig.container.appendChild(politeRegion);
		currentConfig.container.appendChild(assertiveRegion);

		state.initialized = true;
	}

	/**
	 * Get region element for politeness level
	 */
	function getRegion(politeness: LiveRegionPoliteness): HTMLElement | null {
		if (politeness === 'assertive') {
			return assertiveRegion;
		}
		return politeRegion;
	}

	/**
	 * Process announcement queue
	 */
	function processQueue(): void {
		if (isProcessing || queue.length === 0) return;

		isProcessing = true;

		// Sort by priority (higher first)
		queue.sort((a, b) => b.priority - a.priority);

		const announcement = queue.shift();
		if (!announcement) {
			isProcessing = false;
			return;
		}

		state.queueLength = queue.length;

		const region = getRegion(announcement.politeness);
		if (!region) {
			isProcessing = false;
			processQueue();
			return;
		}

		// Clear previous content first (helps screen readers detect change)
		region.textContent = '';

		// Set new content after a brief delay
		setTimeout(() => {
			region.textContent = announcement.message;
			state.lastAnnouncement = announcement.message;

			// Schedule clear
			if (clearTimeoutId !== undefined) {
				clearTimeout(clearTimeoutId);
			}
			clearTimeoutId = setTimeout(() => {
				if (region) {
					region.textContent = '';
				}
			}, currentConfig.clearDelay);

			// Process next announcement after delay
			setTimeout(() => {
				isProcessing = false;
				processQueue();
			}, currentConfig.announcementDelay);
		}, 50);
	}

	/**
	 * Make an announcement
	 */
	function announce(message: string, options: AnnounceOptions = {}): void {
		if (!message.trim()) return;

		// Initialize if needed
		if (!state.initialized) {
			initialize();
		}

		const {
			politeness = currentConfig.politeness,
			clearPrevious = false,
			delay = 0,
			priority = 0,
		} = options;

		// Clear queue if requested
		if (clearPrevious) {
			queue.length = 0;
			state.queueLength = 0;
		}

		// Add to queue
		const queuedAnnouncement: QueuedAnnouncement = {
			message,
			politeness,
			priority,
		};

		if (delay > 0) {
			setTimeout(() => {
				queue.push(queuedAnnouncement);
				state.queueLength = queue.length;
				processQueue();
			}, delay);
		} else {
			queue.push(queuedAnnouncement);
			state.queueLength = queue.length;
			processQueue();
		}
	}

	/**
	 * Make a polite announcement
	 */
	function announcePolite(message: string): void {
		announce(message, { politeness: 'polite' });
	}

	/**
	 * Make an assertive announcement
	 */
	function announceAssertive(message: string): void {
		announce(message, { politeness: 'assertive' });
	}

	/**
	 * Clear current announcements
	 */
	function clear(): void {
		queue.length = 0;
		state.queueLength = 0;

		if (clearTimeoutId !== undefined) {
			clearTimeout(clearTimeoutId);
			clearTimeoutId = undefined;
		}

		if (politeRegion) {
			politeRegion.textContent = '';
		}
		if (assertiveRegion) {
			assertiveRegion.textContent = '';
		}
	}

	/**
	 * Update configuration
	 */
	function updateConfig(newConfig: Partial<LiveRegionConfig>): void {
		currentConfig = {
			...currentConfig,
			...newConfig,
		};

		// Update existing regions if container changed
		if (newConfig.container && state.initialized) {
			// Move regions to new container
			if (politeRegion) {
				newConfig.container.appendChild(politeRegion);
			}
			if (assertiveRegion) {
				newConfig.container.appendChild(assertiveRegion);
			}
		}
	}

	/**
	 * Destroy and clean up
	 */
	function destroy(): void {
		clear();

		if (politeRegion) {
			politeRegion.remove();
			politeRegion = null;
		}

		if (assertiveRegion) {
			assertiveRegion.remove();
			assertiveRegion = null;
		}

		state.initialized = false;
	}

	return {
		state,
		announce,
		announcePolite,
		announceAssertive,
		clear,
		updateConfig,
		destroy,
	};
}

/**
 * Global live region instance for convenience
 */
let globalLiveRegion: LiveRegion | null = null;

/**
 * Get or create global live region
 */
export function getGlobalLiveRegion(): LiveRegion {
	if (!globalLiveRegion) {
		globalLiveRegion = createLiveRegion();
	}
	return globalLiveRegion;
}

/**
 * Make a global announcement
 */
export function announce(message: string, options?: AnnounceOptions): void {
	getGlobalLiveRegion().announce(message, options);
}

/**
 * Make a global polite announcement
 */
export function announcePolite(message: string): void {
	getGlobalLiveRegion().announcePolite(message);
}

/**
 * Make a global assertive announcement
 */
export function announceAssertive(message: string): void {
	getGlobalLiveRegion().announceAssertive(message);
}
