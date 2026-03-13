<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements';
	import { useStableId } from 'src/lib/greater/utils';

	interface Props extends Omit<HTMLTextareaAttributes, 'value' | 'onchange' | 'oninput'> {
		value?: string;
		label?: string;
		helpText?: string;
		errorMessage?: string;
		textareaClass?: string;
		class?: string;
		invalid?: boolean;
		onchange?: (value: string) => void;
		oninput?: (value: string) => void;
	}

	let {
		value = $bindable(''),
		label,
		helpText,
		errorMessage,
		textareaClass = '',
		class: className = '',
		placeholder,
		id,
		required = false,
		disabled = false,
		readonly = false,
		rows = 4,
		maxlength,
		name,
		autocomplete,
		invalid = false,
		onchange,
		oninput,
		style: _style,
		...restProps
	}: Props = $props();

	const stableId = useStableId('textarea');
	const textareaId = $derived(id || stableId.value || undefined);
	const helpTextId = $derived(textareaId ? `${textareaId}-help` : undefined);
	const errorId = $derived(textareaId ? `${textareaId}-error` : undefined);

	const isInvalid = $derived(invalid || Boolean(errorMessage));

	function handleInput(event: Event) {
		const target = event.target as HTMLTextAreaElement;
		value = target.value;
		oninput?.(value);
	}

	function handleChange(event: Event) {
		const target = event.target as HTMLTextAreaElement;
		value = target.value;
		onchange?.(value);
	}
</script>

<div class={`gr-textarea-field ${className}`.trim()}>
	{#if label}
		<label
			for={textareaId}
			class="gr-textarea__label"
			class:gr-textarea__label--required={required}
		>
			{label}
			{#if required}
				<span class="gr-textarea__required" aria-hidden="true">*</span>
			{/if}
		</label>
	{/if}

	<textarea
		id={textareaId}
		bind:value
		{placeholder}
		{disabled}
		{readonly}
		{required}
		{rows}
		{maxlength}
		{name}
		{autocomplete}
		class={`gr-textarea ${textareaClass}`.trim()}
		aria-invalid={isInvalid || undefined}
		aria-describedby={[
			helpText && !isInvalid ? helpTextId : null,
			errorMessage && isInvalid ? errorId : null,
		]
			.filter(Boolean)
			.join(' ') || undefined}
		oninput={handleInput}
		onchange={handleChange}
		{...restProps}
	></textarea>

	{#if helpText && !isInvalid}
		<div id={helpTextId} class="gr-textarea__help">
			{helpText}
		</div>
	{/if}

	{#if errorMessage && isInvalid}
		<div id={errorId} class="gr-textarea__error" role="alert" aria-live="polite">
			{errorMessage}
		</div>
	{/if}
</div>
