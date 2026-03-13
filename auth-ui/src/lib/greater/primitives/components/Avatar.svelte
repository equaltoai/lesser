<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { useStableId } from 'src/lib/greater/utils';
	import defaultAvatar from '../assets/greater-default-profile.png';
	import Spinner from './Spinner.svelte';

	// Map avatar size to spinner size
	const avatarSpinnerSizeMap: Record<string, 'xs' | 'sm' | 'md' | 'lg'> = {
		xs: 'xs',
		sm: 'xs',
		md: 'sm',
		lg: 'sm',
		xl: 'md',
		'2xl': 'lg',
	};

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'role'> {
		src?: string;
		alt?: string;
		name?: string;
		/**
		 * Text label fallback for chat interfaces (e.g., "You", "PAI", "AI")
		 * Used when fallbackMode is 'label'
		 */
		label?: string;
		/**
		 * Icon snippet fallback for avatar
		 * Used when fallbackMode is 'icon'
		 */
		labelIcon?: Snippet;
		/**
		 * Controls which fallback to use when image is not available
		 * - 'initials': Show initials from name prop (default)
		 * - 'label': Show label text
		 * - 'icon': Show labelIcon or default user icon
		 * @default 'initials'
		 */
		fallbackMode?: 'initials' | 'label' | 'icon';
		size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
		shape?: 'circle' | 'square' | 'rounded';
		loading?: boolean;
		status?: 'online' | 'offline' | 'busy' | 'away';
		statusPosition?: 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left';
		class?: string;
		fallback?: Snippet;
		role?: string;
	}

	let {
		src,
		alt,
		name = '',
		label,
		labelIcon,
		fallbackMode = 'initials',
		size = 'md',
		shape = 'circle',
		loading = false,
		status,
		statusPosition = 'bottom-right',
		class: className = '',
		fallback,
		id,
		onclick,
		onmouseenter,
		onmouseleave,
		onfocus,
		onblur,
		onkeydown,
		onkeyup,
		role,
		'aria-label': ariaLabel,
		'aria-labelledby': ariaLabelledby,
		'aria-describedby': ariaDescribedby,
		tabindex,
	}: Props = $props();

	const INTERACTIVE_ROLES = new Set([
		'button',
		'checkbox',
		'combobox',
		'link',
		'menuitem',
		'menuitemcheckbox',
		'menuitemradio',
		'option',
		'radio',
		'searchbox',
		'slider',
		'spinbutton',
		'switch',
		'tab',
		'textbox',
		'treeitem',
	]);

	const parsedTabIndex = $derived.by<number | undefined>(() => {
		if (tabindex === undefined || tabindex === null) {
			return undefined;
		}
		if (typeof tabindex === 'number') {
			return Number.isFinite(tabindex) ? tabindex : undefined;
		}
		const numericValue = Number(tabindex);
		return Number.isFinite(numericValue) ? numericValue : undefined;
	});

	const hasInteractiveHandlers = $derived.by(() => Boolean(onclick || onkeydown || onkeyup));

	const isInteractiveRole = (roleValue: string | undefined): boolean => {
		if (!roleValue) {
			return false;
		}
		return INTERACTIVE_ROLES.has(roleValue);
	};

	const isInteractive = $derived.by(() => {
		if (isInteractiveRole(role)) {
			return true;
		}
		if (hasInteractiveHandlers) {
			return true;
		}
		if (parsedTabIndex !== undefined && parsedTabIndex >= 0) {
			return true;
		}
		return false;
	});

	// State management
	let imageLoaded = $state(false);
	let imageError = $state(false);
	let imageElement: HTMLImageElement | null = $state(null);

	// Compute avatar classes
	const avatarClass = $derived.by(() => {
		const classes = [
			'gr-avatar',
			`gr-avatar--${size}`,
			`gr-avatar--${shape}`,
			loading && 'gr-avatar--loading',
			status && 'gr-avatar--has-status',
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	// Compute status classes
	const statusClass = $derived.by(() => {
		if (!status) return '';

		const classes = [
			'gr-avatar__status',
			`gr-avatar__status--${status}`,
			`gr-avatar__status--${statusPosition}`,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	// Generate initials from name
	const initials = $derived.by(() => {
		if (!name) return '';

		const words = name.trim().split(/\s+/);
		if (words.length === 1) {
			// Single word - take first two characters
			const firstWord = words[0];
			if (!firstWord) return '';
			return firstWord.substring(0, 2).toUpperCase();
		} else {
			// Multiple words - take first character of first two words
			return words
				.slice(0, 2)
				.map((word) => word.charAt(0))
				.join('')
				.toUpperCase();
		}
	});

	// Generate deterministic color class from name or label
	function generateColorClass(colorSource: string): string {
		if (!colorSource) return '';

		// Simple hash function to generate consistent color
		let hash = 0;
		for (let i = 0; i < colorSource.length; i++) {
			hash = colorSource.charCodeAt(i) + ((hash << 5) - hash);
		}

		// Map hash to color index (0-11)
		const index = Math.abs(hash) % 12;
		return `gr-avatar--color-${index}`;
	}

	// Generate color class from name or label
	const colorClass = $derived.by(() => {
		const colorSource = name || label;
		if (!colorSource) return '';
		return generateColorClass(colorSource);
	});

	function handleImageLoad() {
		imageLoaded = true;
		imageError = false;
	}

	function handleImageError() {
		imageLoaded = false;
		imageError = true;
	}

	// Reset image state when src changes
	$effect(() => {
		if (src) {
			imageLoaded = false;
			imageError = false;
		}
	});

	// Compute accessible name
	const accessibleName = $derived.by(() => {
		if (alt) return alt;
		if (name) return name;
		if (label) return label;
		return 'Avatar';
	});

	// Generate unique ID for status
	const generatedStatusId = useStableId('avatar-status');
	const statusId = $derived(generatedStatusId.value);
</script>

{#snippet AvatarContent()}
	{#if loading}
		<div class="gr-avatar__loading" aria-busy="true">
			<Spinner size={avatarSpinnerSizeMap[size] || 'sm'} color="current" label="Loading avatar" />
		</div>
	{:else if src && !imageError}
		<img
			bind:this={imageElement}
			class="gr-avatar__image {imageLoaded ? 'gr-avatar__image--loaded' : ''}"
			{src}
			alt={alt || name || label || 'Avatar'}
			onload={handleImageLoad}
			onerror={handleImageError}
		/>

		{#if !imageLoaded}
			{@const computedInitials = initials}
			<div class="gr-avatar__placeholder" aria-hidden="true">
				{#if fallback}
					{@render fallback()}
				{:else if fallbackMode === 'label' && label}
					<span class="gr-avatar__label {colorClass}">
						{label}
					</span>
				{:else if fallbackMode === 'icon'}
					<span class="gr-avatar__icon {colorClass}">
						{#if labelIcon}
							{@render labelIcon()}
						{:else}
							<svg
								class="gr-avatar__default-icon"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
							>
								<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
								<circle cx="12" cy="7" r="4" />
							</svg>
						{/if}
					</span>
				{:else if fallbackMode === 'initials' && computedInitials}
					<span class="gr-avatar__initials {colorClass}">
						{computedInitials}
					</span>
				{:else}
					<img
						class="gr-avatar__fallback-image"
						src={defaultAvatar}
						alt="Default avatar"
						loading="lazy"
					/>
				{/if}
			</div>
		{/if}
	{:else}
		<!-- No image src or image failed to load -->
		{@const computedInitials = initials}
		<div class="gr-avatar__placeholder" aria-hidden="true">
			{#if fallback}
				<!-- Custom fallback snippet takes highest priority -->
				{@render fallback()}
			{:else if fallbackMode === 'label' && label}
				<!-- Label fallback mode -->
				<span class="gr-avatar__label {colorClass}">
					{label}
				</span>
			{:else if fallbackMode === 'icon'}
				<!-- Icon fallback mode -->
				<span class="gr-avatar__icon {colorClass}">
					{#if labelIcon}
						{@render labelIcon()}
					{:else}
						<!-- Default user icon -->
						<svg
							class="gr-avatar__default-icon"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
							<circle cx="12" cy="7" r="4" />
						</svg>
					{/if}
				</span>
			{:else if fallbackMode === 'initials' && computedInitials}
				<!-- Initials fallback mode (default) -->
				<span class="gr-avatar__initials {colorClass}">
					{computedInitials}
				</span>
			{:else}
				<!-- Ultimate fallback: default avatar image -->
				<img
					class="gr-avatar__fallback-image"
					src={defaultAvatar}
					alt="Default avatar"
					loading="lazy"
				/>
			{/if}
		</div>
	{/if}

	{#if status}
		<div class={statusClass} id={statusId} role="status" aria-label={`Status: ${status}`}></div>
	{/if}
{/snippet}

{#if isInteractive}
	<button
		class={avatarClass}
		aria-label={ariaLabel ?? accessibleName}
		aria-labelledby={ariaLabelledby}
		aria-describedby={ariaDescribedby ?? (status ? statusId : undefined)}
		{id}
		{onclick}
		{onmouseenter}
		{onmouseleave}
		{onfocus}
		{onblur}
		{onkeydown}
		{onkeyup}
		{role}
		tabindex={parsedTabIndex}
		type="button"
	>
		{@render AvatarContent()}
	</button>
{:else}
	<div
		class={avatarClass}
		role={role ?? 'img'}
		aria-label={ariaLabel ?? accessibleName}
		aria-labelledby={ariaLabelledby}
		aria-describedby={ariaDescribedby ?? (status ? statusId : undefined)}
		{id}
		{onclick}
		{onmouseenter}
		{onmouseleave}
		{onfocus}
		{onblur}
		{onkeydown}
		{onkeyup}
	>
		{@render AvatarContent()}
	</div>
{/if}
