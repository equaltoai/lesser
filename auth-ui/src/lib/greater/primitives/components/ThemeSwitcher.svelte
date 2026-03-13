<script lang="ts">
	import type { Snippet } from 'svelte';
	import {
		preferencesStore,
		type ColorScheme,
		type Density,
		type FontSize,
		type MotionPreference,
	} from '../stores/preferences';

	interface Props {
		variant?: 'compact' | 'full';
		showPreview?: boolean;
		/** @deprecated showAdvanced is deprecated and will be removed in a future version. Custom colors require external CSS for CSP compliance. */
		showAdvanced?: boolean;
		/** @deprecated showWorkbench is deprecated and will be removed in a future version. Use the standalone ThemeWorkbench component for development purposes only. */
		showWorkbench?: boolean;
		class?: string;
		value?: ColorScheme;
		onThemeChange?: (theme: ColorScheme) => void;
		children?: Snippet;
	}

	let {
		variant = 'compact', // Default to compact for header usage
		showPreview = true,
		showAdvanced: _showAdvanced = false,
		showWorkbench: _showWorkbench = false,
		class: className = '',
		value,
		onThemeChange,
		children,
	}: Props = $props();

	// Get reactive state from preferences store
	let preferences = $state(preferencesStore.preferences);
	let preferencesState = $state(preferencesStore.state);

	// Use value prop if provided, otherwise use store
	const currentScheme = $derived(value ?? preferences.colorScheme);

	// Compact dropdown state
	let isCompactOpen = $state(false);
	let compactTrigger: HTMLElement | null = $state(null);
	let compactMenu: HTMLElement | null = $state(null);

	function syncPreferences() {
		preferences = preferencesStore.preferences;
		preferencesState = preferencesStore.state;
	}

	syncPreferences();

	// Theme options
	const colorSchemes: { value: ColorScheme; label: string; description: string }[] = [
		{ value: 'light', label: 'Light', description: 'Light background with dark text' },
		{ value: 'dark', label: 'Dark', description: 'Dark background with light text' },
		{
			value: 'high-contrast',
			label: 'High Contrast',
			description: 'Maximum contrast for accessibility',
		},
		{ value: 'auto', label: 'Auto', description: 'Follow system preference' },
	];

	const densities: { value: Density; label: string; description: string }[] = [
		{ value: 'compact', label: 'Compact', description: 'Reduced spacing for more content' },
		{ value: 'comfortable', label: 'Comfortable', description: 'Balanced spacing' },
		{ value: 'spacious', label: 'Spacious', description: 'Extra spacing for readability' },
	];

	const fontSizes: { value: FontSize; label: string; scale: number }[] = [
		{ value: 'small', label: 'Small', scale: 0.875 },
		{ value: 'medium', label: 'Medium', scale: 1 },
		{ value: 'large', label: 'Large', scale: 1.125 },
	];

	const motionPreferences: { value: MotionPreference; label: string; description: string }[] = [
		{ value: 'normal', label: 'Normal', description: 'Full animations and transitions' },
		{ value: 'reduced', label: 'Reduced', description: 'Minimal motion for accessibility' },
	];

	const hasDocument = typeof document !== 'undefined';

	// Handlers
	function handleColorSchemeChange(scheme: ColorScheme) {
		if (value === undefined) {
			preferencesStore.setColorScheme(scheme);
		}
		onThemeChange?.(scheme);
		if (variant === 'compact') {
			isCompactOpen = false;
		}

		syncPreferences();
	}

	function handleDensityChange(density: Density) {
		preferencesStore.setDensity(density);
		syncPreferences();
	}

	function handleFontSizeChange(size: FontSize) {
		preferencesStore.setFontSize(size);
		syncPreferences();
	}

	function handleMotionChange(motion: MotionPreference) {
		preferencesStore.setMotion(motion);
		syncPreferences();
	}

	function handleHighContrastToggle() {
		preferencesStore.setHighContrastMode(!preferences.highContrastMode);
		syncPreferences();
	}

	function resetToDefaults() {
		preferencesStore.reset();
		syncPreferences();
	}

	// Compact variant handlers
	function toggleCompact() {
		isCompactOpen = !isCompactOpen;
	}

	function closeCompact() {
		isCompactOpen = false;
	}

	// Close compact menu when clicking outside
	$effect(() => {
		if (!hasDocument || !isCompactOpen || variant !== 'compact') return;

		function handleClickOutside(event: MouseEvent) {
			if (
				compactMenu &&
				compactTrigger &&
				!compactMenu.contains(event.target as Node) &&
				!compactTrigger.contains(event.target as Node)
			) {
				closeCompact();
			}
		}

		function handleEscape(event: KeyboardEvent) {
			if (event.key === 'Escape') {
				closeCompact();
				compactTrigger?.focus();
			}
		}

		document.addEventListener('click', handleClickOutside);
		document.addEventListener('keydown', handleEscape);

		return () => {
			document.removeEventListener('click', handleClickOutside);
			document.removeEventListener('keydown', handleEscape);
		};
	});

	// Get label for current scheme
	const currentSchemeLabel = $derived(() => {
		const scheme = colorSchemes.find((s) => s.value === currentScheme);
		return scheme?.label ?? 'Auto';
	});

	// Filter color schemes for compact (only show Light, Dark, Auto)
	const compactSchemes = $derived(() => {
		return colorSchemes.filter((s) => ['light', 'dark', 'auto'].includes(s.value));
	});
</script>

<div class="gr-theme-switcher gr-theme-switcher--{variant} {className}">
	{#if variant === 'compact'}
		<!-- Compact variant: dropdown button -->
		<div class="gr-theme-switcher__compact">
			<button
				bind:this={compactTrigger}
				onclick={toggleCompact}
				class="gr-theme-switcher__compact-button"
				aria-expanded={isCompactOpen}
				aria-haspopup="menu"
				type="button"
			>
				<span class="gr-theme-switcher__compact-label">{currentSchemeLabel()}</span>
				<svg
					width="16"
					height="16"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					class="gr-theme-switcher__compact-icon"
					class:gr-theme-switcher__compact-icon--open={isCompactOpen}
				>
					<polyline points="6 9 12 15 18 9"></polyline>
				</svg>
			</button>

			{#if isCompactOpen}
				<div bind:this={compactMenu} class="gr-theme-switcher__compact-menu" role="menu">
					{#each compactSchemes() as scheme (scheme.value)}
						<button
							class="gr-theme-switcher__compact-menu-item"
							class:gr-theme-switcher__compact-menu-item--active={currentScheme === scheme.value}
							role="menuitemradio"
							aria-checked={currentScheme === scheme.value}
							onclick={() => handleColorSchemeChange(scheme.value)}
							type="button"
						>
							<span class="gr-theme-switcher__compact-menu-label">{scheme.label}</span>
							{#if currentScheme === scheme.value}
								<svg
									width="16"
									height="16"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									class="gr-theme-switcher__compact-menu-check"
								>
									<polyline points="20 6 9 17 4 12"></polyline>
								</svg>
							{/if}
						</button>
					{/each}
				</div>
			{/if}
		</div>
	{:else}
		<!-- Full variant: settings panel -->
		<div class="gr-theme-switcher__section">
			<h3 class="gr-theme-switcher__heading">Color Scheme</h3>
			<div class="gr-theme-switcher__options">
				{#each colorSchemes as scheme (scheme.value)}
					<label class="gr-theme-switcher__option">
						<input
							type="radio"
							name="colorScheme"
							value={scheme.value}
							checked={currentScheme === scheme.value}
							onchange={() => handleColorSchemeChange(scheme.value)}
							class="gr-theme-switcher__radio"
						/>
						<div class="gr-theme-switcher__option-content">
							<span class="gr-theme-switcher__option-label">{scheme.label}</span>
							<span class="gr-theme-switcher__option-description">{scheme.description}</span>
							{#if scheme.value === 'auto'}
								<span class="gr-theme-switcher__option-badge">
									Currently: {preferencesState.systemColorScheme}
								</span>
							{/if}
						</div>
					</label>
				{/each}
			</div>

			<label class="gr-theme-switcher__checkbox-label">
				<input
					type="checkbox"
					checked={preferences.highContrastMode}
					onchange={handleHighContrastToggle}
					class="gr-theme-switcher__checkbox"
				/>
				<span>Force high contrast mode</span>
			</label>
		</div>

		<div class="gr-theme-switcher__section">
			<h3 class="gr-theme-switcher__heading">Density</h3>
			<div class="gr-theme-switcher__options">
				{#each densities as density (density.value)}
					<label class="gr-theme-switcher__option">
						<input
							type="radio"
							name="density"
							value={density.value}
							checked={preferences.density === density.value}
							onchange={() => handleDensityChange(density.value)}
							class="gr-theme-switcher__radio"
						/>
						<div class="gr-theme-switcher__option-content">
							<span class="gr-theme-switcher__option-label">{density.label}</span>
							<span class="gr-theme-switcher__option-description">{density.description}</span>
						</div>
					</label>
				{/each}
			</div>
		</div>

		<div class="gr-theme-switcher__section">
			<h3 class="gr-theme-switcher__heading">Font Size</h3>
			<div class="gr-theme-switcher__options">
				{#each fontSizes as size (size.value)}
					<label class="gr-theme-switcher__option">
						<input
							type="radio"
							name="fontSize"
							value={size.value}
							checked={preferences.fontSize === size.value}
							onchange={() => handleFontSizeChange(size.value)}
							class="gr-theme-switcher__radio"
						/>
						<div class="gr-theme-switcher__option-content">
							<span class="gr-theme-switcher__option-label">{size.label}</span>
						</div>
					</label>
				{/each}
			</div>
		</div>

		<div class="gr-theme-switcher__section">
			<h3 class="gr-theme-switcher__heading">Motion</h3>
			<div class="gr-theme-switcher__options">
				{#each motionPreferences as motion (motion.value)}
					<label class="gr-theme-switcher__option">
						<input
							type="radio"
							name="motion"
							value={motion.value}
							checked={preferences.motion === motion.value}
							disabled={preferencesState.systemMotion === 'reduced'}
							onchange={() => handleMotionChange(motion.value)}
							class="gr-theme-switcher__radio"
						/>
						<div class="gr-theme-switcher__option-content">
							<span class="gr-theme-switcher__option-label">{motion.label}</span>
							<span class="gr-theme-switcher__option-description">{motion.description}</span>
							{#if preferencesState.systemMotion === 'reduced' && motion.value === 'normal'}
								<span class="gr-theme-switcher__option-badge"> System prefers reduced motion </span>
							{/if}
						</div>
					</label>
				{/each}
			</div>
		</div>

		{#if showPreview}
			<div class="gr-theme-switcher__section">
				<h3 class="gr-theme-switcher__heading">Preview</h3>
				<div class="gr-theme-switcher__preview">
					<div class="gr-theme-switcher__preview-card">
						<h4>Sample Card</h4>
						<p>This is a preview of how your theme looks.</p>
						<div class="gr-theme-switcher__preview-buttons">
							<button
								class="gr-theme-switcher__preview-button gr-theme-switcher__preview-button--primary"
							>
								Primary
							</button>
							<button
								class="gr-theme-switcher__preview-button gr-theme-switcher__preview-button--secondary"
							>
								Secondary
							</button>
						</div>
					</div>
				</div>
			</div>
		{/if}

		<div class="gr-theme-switcher__actions">
			<button
				onclick={resetToDefaults}
				class="gr-theme-switcher__action-button gr-theme-switcher__action-button--secondary"
			>
				Reset to Defaults
			</button>
		</div>

		{#if children}
			<div class="gr-theme-switcher__custom">
				{@render children()}
			</div>
		{/if}
	{/if}
</div>
