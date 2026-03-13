<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { useStableId } from 'src/lib/greater/utils';

	interface Props extends Omit<HTMLInputAttributes, 'type' | 'value' | 'prefix'> {
		label?: string;
		value?: string;
		type?: 'text' | 'email' | 'password' | 'url' | 'tel' | 'search';
		placeholder?: string;
		invalid?: boolean;
		disabled?: boolean;
		readonly?: boolean;
		required?: boolean;
		helpText?: string;
		errorMessage?: string;
		class?: string;
		inputClass?: string;
		prefix?: Snippet;
		suffix?: Snippet;
	}

	let {
		label,
		value = $bindable(''),
		type = 'text',
		placeholder,
		invalid = false,
		disabled = false,
		readonly = false,
		required = false,
		helpText,
		errorMessage,
		class: className = '',
		inputClass = '',
		prefix,
		suffix,
		id,
		onblur,
		onfocus,
		oninput,
		onkeydown,
		style: _style,
		...restProps
	}: Props = $props();

	// Generate unique ID for accessibility
	const generatedId = useStableId('textfield');
	const fieldId = $derived(id || generatedId.value);
	const helpTextId = $derived(`${fieldId}-help`);
	const errorId = $derived(`${fieldId}-error`);

	let focused = $state(false);
	let hasValue = $derived(Boolean(value));

	// Compute field classes
	const fieldClass = $derived.by(() => {
		const classes = [
			'gr-textfield',
			focused && 'gr-textfield--focused',
			invalid && 'gr-textfield--invalid',
			disabled && 'gr-textfield--disabled',
			readonly && 'gr-textfield--readonly',
			hasValue && 'gr-textfield--has-value',
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	const inputClasses = $derived.by(() => {
		const classes = ['gr-textfield__input', inputClass].filter(Boolean).join(' ');

		return classes;
	});

	function handleFocus(event: FocusEvent & { currentTarget: EventTarget & HTMLInputElement }) {
		focused = true;
		onfocus?.(event);
	}

	function handleBlur(event: FocusEvent & { currentTarget: EventTarget & HTMLInputElement }) {
		focused = false;
		onblur?.(event);
	}

	function handleInput(event: Event & { currentTarget: EventTarget & HTMLInputElement }) {
		value = event.currentTarget.value;
		oninput?.(event);
	}

	function handleKeydown(event: KeyboardEvent & { currentTarget: EventTarget & HTMLInputElement }) {
		onkeydown?.(event);
	}
</script>

<div class={fieldClass}>
	{#if label}
		<label for={fieldId} class="gr-textfield__label" class:gr-textfield__label--required={required}>
			{label}
			{#if required}
				<span class="gr-textfield__required" aria-hidden="true">*</span>
			{/if}
		</label>
	{/if}

	<div class="gr-textfield__container">
		{#if prefix}
			<div class="gr-textfield__prefix" aria-hidden="true">
				{@render prefix()}
			</div>
		{/if}

		<input
			{type}
			id={fieldId}
			class={inputClasses}
			{placeholder}
			bind:value
			{disabled}
			{readonly}
			{required}
			aria-invalid={invalid}
			aria-describedby={[helpText ? helpTextId : null, errorMessage && invalid ? errorId : null]
				.filter(Boolean)
				.join(' ') || undefined}
			onfocus={handleFocus}
			onblur={handleBlur}
			oninput={handleInput}
			onkeydown={handleKeydown}
			{...restProps}
		/>

		{#if suffix}
			<div class="gr-textfield__suffix" aria-hidden="true">
				{@render suffix()}
			</div>
		{/if}
	</div>

	{#if helpText && !invalid}
		<div id={helpTextId} class="gr-textfield__help">
			{helpText}
		</div>
	{/if}

	{#if errorMessage && invalid}
		<div id={errorId} class="gr-textfield__error" role="alert" aria-live="polite">
			{errorMessage}
		</div>
	{/if}
</div>
