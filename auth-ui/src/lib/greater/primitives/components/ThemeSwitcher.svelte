<script lang="ts">
	import type { Snippet } from 'svelte';
	import {
		preferencesStore,
		type ColorScheme,
		type Density,
		type FontSize,
		type MotionPreference,
	} from '../stores/preferences';
	import ThemeWorkbench from './Theme/ThemeWorkbench.svelte';

	interface Props {
		variant?: 'compact' | 'full';
		showPreview?: boolean;
		showAdvanced?: boolean;
		showWorkbench?: boolean;
		class?: string;
		value?: ColorScheme;
		onThemeChange?: (theme: ColorScheme) => void;
		children?: Snippet;
	}

	let {
		variant = 'compact', // Default to compact for header usage
		showPreview = true,
		showAdvanced = false,
		showWorkbench = false,
		class: className = '',
		value,
		onThemeChange,
		children,
	}: Props = $props();

	// Get reactive state from preferences store
	let preferences = $state(preferencesStore.preferences);
	let state = $state(preferencesStore.state);

	// Use value prop if provided, otherwise use store
	const currentScheme = $derived(value ?? preferences.colorScheme);

	// Compact dropdown state
	let isCompactOpen = $state(false);
	let compactTrigger: HTMLElement | null = $state(null);
	let compactMenu: HTMLElement | null = $state(null);

	// Local state for color picker
	let primaryColor = $state('#3b82f6');
	let secondaryColor = $state('#8b5cf6');
	let accentColor = $state('#ec4899');
	let fontScale = $state(1);

	function hexToRgb(hex: string) {
		const normalized = hex.replace('#', '').trim();
		if (normalized.length !== 6) {
			return null;
		}

		const r = Number.parseInt(normalized.slice(0, 2), 16) / 255;
		const g = Number.parseInt(normalized.slice(2, 4), 16) / 255;
		const b = Number.parseInt(normalized.slice(4, 6), 16) / 255;
		return { r, g, b };
	}

	function relativeLuminance(rgb: { r: number; g: number; b: number }) {
		const transform = (value: number) =>
			value <= 0.03928 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4);
		const r = transform(rgb.r);
		const g = transform(rgb.g);
		const b = transform(rgb.b);
		return 0.2126 * r + 0.7152 * g + 0.0722 * b;
	}

	function getContrastRatio(hexA: string, hexB: string) {
		const a = hexToRgb(hexA);
		const b = hexToRgb(hexB);
		if (!a || !b) {
			return 1;
		}

		const lumA = relativeLuminance(a);
		const lumB = relativeLuminance(b);
		const lighter = Math.max(lumA, lumB);
		const darker = Math.min(lumA, lumB);
		return (lighter + 0.05) / (darker + 0.05);
	}

	function syncPreferences() {
		preferences = preferencesStore.preferences;
		state = preferencesStore.state;
		primaryColor = preferences.customColors.primary;
		secondaryColor = preferences.customColors.secondary;
		accentColor = preferences.customColors.accent || '#ec4899';
		fontScale = preferences.fontScale;
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

	function handleFontScaleChange(scale: number) {
		fontScale = scale;
		preferencesStore.setFontScale(scale);
		syncPreferences();
	}

	function handleMotionChange(motion: MotionPreference) {
		preferencesStore.setMotion(motion);
		syncPreferences();
	}

	function handleColorChange(type: 'primary' | 'secondary' | 'accent', color: string) {
		switch (type) {
			case 'primary':
				primaryColor = color;
				break;
			case 'secondary':
				secondaryColor = color;
				break;
			case 'accent':
				accentColor = color;
				break;
		}

		preferencesStore.setCustomColors({
			primary: primaryColor,
			secondary: secondaryColor,
			accent: accentColor,
		});
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

	function exportSettings() {
		if (!hasDocument) {
			return;
		}

		const json = preferencesStore.export();
		const blob = new Blob([json], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'theme-preferences.json';
		a.click();
		URL.revokeObjectURL(url);
	}

	function importSettings(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		const reader = new FileReader();
		reader.onload = (e) => {
			const json = e.target?.result as string;
			if (preferencesStore.import(json)) {
				syncPreferences();
			} else {
				alert('Invalid preferences file');
			}
		};
		reader.readAsText(file);
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

	const previewButtonTextColor = $derived(() => {
		const contrastWithBlack = getContrastRatio(primaryColor, '#000000');
		const contrastWithWhite = getContrastRatio(primaryColor, '#ffffff');
		return contrastWithBlack >= contrastWithWhite ? '#000000' : '#ffffff';
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
									Currently: {state.systemColorScheme}
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

			{#if showAdvanced}
				<div class="gr-theme-switcher__slider">
					<label for="fontScale">
						Font Scale: {fontScale.toFixed(2)}x
					</label>
					<input
						id="fontScale"
						type="range"
						min="0.85"
						max="1.5"
						step="0.05"
						value={fontScale}
						oninput={(e) => handleFontScaleChange(Number(e.currentTarget.value))}
						class="gr-theme-switcher__range"
					/>
				</div>
			{/if}
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
							disabled={state.systemMotion === 'reduced'}
							onchange={() => handleMotionChange(motion.value)}
							class="gr-theme-switcher__radio"
						/>
						<div class="gr-theme-switcher__option-content">
							<span class="gr-theme-switcher__option-label">{motion.label}</span>
							<span class="gr-theme-switcher__option-description">{motion.description}</span>
							{#if state.systemMotion === 'reduced' && motion.value === 'normal'}
								<span class="gr-theme-switcher__option-badge"> System prefers reduced motion </span>
							{/if}
						</div>
					</label>
				{/each}
			</div>
		</div>

		{#if showAdvanced}
			<div class="gr-theme-switcher__section">
				<h3 class="gr-theme-switcher__heading">Custom Colors</h3>
				<div class="gr-theme-switcher__colors">
					<div class="gr-theme-switcher__color-input">
						<label for="primaryColor">Primary</label>
						<div class="gr-theme-switcher__color-wrapper">
							<input
								id="primaryColor"
								type="color"
								value={primaryColor}
								oninput={(e) => handleColorChange('primary', e.currentTarget.value)}
								class="gr-theme-switcher__color-picker"
							/>
							<input
								type="text"
								value={primaryColor}
								oninput={(e) => handleColorChange('primary', e.currentTarget.value)}
								pattern="^#[0-9A-Fa-f]{6}$"
								aria-label="Primary color hex value"
								class="gr-theme-switcher__color-text"
							/>
						</div>
					</div>

					<div class="gr-theme-switcher__color-input">
						<label for="secondaryColor">Secondary</label>
						<div class="gr-theme-switcher__color-wrapper">
							<input
								id="secondaryColor"
								type="color"
								value={secondaryColor}
								oninput={(e) => handleColorChange('secondary', e.currentTarget.value)}
								class="gr-theme-switcher__color-picker"
							/>
							<input
								type="text"
								value={secondaryColor}
								oninput={(e) => handleColorChange('secondary', e.currentTarget.value)}
								pattern="^#[0-9A-Fa-f]{6}$"
								aria-label="Secondary color hex value"
								class="gr-theme-switcher__color-text"
							/>
						</div>
					</div>

					<div class="gr-theme-switcher__color-input">
						<label for="accentColor">Accent</label>
						<div class="gr-theme-switcher__color-wrapper">
							<input
								id="accentColor"
								type="color"
								value={accentColor}
								oninput={(e) => handleColorChange('accent', e.currentTarget.value)}
								class="gr-theme-switcher__color-picker"
							/>
							<input
								type="text"
								value={accentColor}
								oninput={(e) => handleColorChange('accent', e.currentTarget.value)}
								pattern="^#[0-9A-Fa-f]{6}$"
								aria-label="Accent color hex value"
								class="gr-theme-switcher__color-text"
							/>
						</div>
					</div>
				</div>
			</div>
		{/if}

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
								style={`background-color: ${primaryColor}; color: ${previewButtonTextColor()}`}
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

			{#if showAdvanced}
				<button
					onclick={exportSettings}
					class="gr-theme-switcher__action-button gr-theme-switcher__action-button--secondary"
				>
					Export Settings
				</button>

				<label class="gr-theme-switcher__action-button gr-theme-switcher__action-button--secondary">
					Import Settings
					<input
						type="file"
						accept=".json"
						onchange={importSettings}
						class="gr-theme-switcher__file-input"
					/>
				</label>
			{/if}
		</div>

		{#if showWorkbench}
			<div class="gr-theme-switcher__section">
				<h3 class="gr-theme-switcher__heading">Theme Workbench</h3>
				<ThemeWorkbench
					initialColor={primaryColor}
					onSave={(theme) => {
						// This would typically save to a custom theme store
						handleColorChange('primary', theme.colors.primary[500]);
						handleColorChange('secondary', theme.colors.secondary[500]);
						// Example: just applying primary/secondary for now
					}}
				/>
			</div>
		{/if}

		{#if children}
			<div class="gr-theme-switcher__custom">
				{@render children()}
			</div>
		{/if}
	{/if}
</div>
