/**
 * TextField Primitive
 *
 * Headless primitive for text input fields with validation, formatting, and accessibility.
 * Provides behavior for single-line and multi-line text inputs with error handling.
 *
 * @module primitives/textfield
 */

/**
 * TextField configuration
 */
export interface TextFieldConfig {
	/**
	 * Initial value
	 */
	value?: string;

	/**
	 * Placeholder text
	 */
	placeholder?: string;

	/**
	 * Whether the field is disabled
	 */
	disabled?: boolean;

	/**
	 * Whether the field is required
	 */
	required?: boolean;

	/**
	 * Whether the field is read-only
	 */
	readonly?: boolean;

	/**
	 * Whether the field is multiline (textarea)
	 */
	multiline?: boolean;

	/**
	 * Maximum character length
	 */
	maxLength?: number;

	/**
	 * Minimum character length
	 */
	minLength?: number;

	/**
	 * Pattern for validation (regex)
	 */
	pattern?: string;

	/**
	 * Input type (text, email, url, tel, password, etc.)
	 */
	type?: string;

	/**
	 * Autocomplete behavior
	 */
	autocomplete?: string;

	/**
	 * ARIA label
	 */
	label?: string;

	/**
	 * ARIA described by (for error messages)
	 */
	describedBy?: string;

	/**
	 * Validation function
	 */
	validate?: (value: string) => string | null;

	/**
	 * Change handler
	 */
	onChange?: (value: string) => void;

	/**
	 * Focus handler
	 */
	onFocus?: () => void;

	/**
	 * Blur handler
	 */
	onBlur?: () => void;

	/**
	 * Submit handler (for Enter key)
	 */
	onSubmit?: (value: string) => void;

	/**
	 * Cleanup handler
	 */
	onDestroy?: () => void;
}

/**
 * TextField state
 */
export interface TextFieldState {
	/**
	 * Current value
	 */
	value: string;

	/**
	 * Whether the field is focused
	 */
	focused: boolean;

	/**
	 * Whether the field has been touched
	 */
	touched: boolean;

	/**
	 * Whether the field is valid
	 */
	valid: boolean;

	/**
	 * Current error message
	 */
	error: string | null;

	/**
	 * Character count
	 */
	length: number;

	/**
	 * Whether max length is exceeded
	 */
	exceededLength: boolean;

	/**
	 * Whether the field is disabled
	 */
	disabled: boolean;

	/**
	 * Whether the field is required
	 */
	required: boolean;

	/**
	 * Whether the field is read-only
	 */
	readonly: boolean;
}

/**
 * TextField actions
 */
export interface TextFieldActions {
	/**
	 * Svelte action for input/textarea elements
	 */
	field: (node: HTMLInputElement | HTMLTextAreaElement) => { destroy: () => void };
}

/**
 * TextField helpers
 */
export interface TextFieldHelpers {
	/**
	 * Set the value
	 */
	setValue: (value: string) => void;

	/**
	 * Clear the value
	 */
	clear: () => void;

	/**
	 * Validate the current value
	 */
	validate: () => boolean;

	/**
	 * Mark the field as touched
	 */
	markTouched: () => void;

	/**
	 * Focus the field
	 */
	focus: () => void;

	/**
	 * Blur the field
	 */
	blur: () => void;
}

/**
 * TextField context
 */
export interface TextField {
	/**
	 * Current state
	 */
	state: TextFieldState;

	/**
	 * Actions to apply to DOM elements
	 */
	actions: TextFieldActions;

	/**
	 * Helper functions
	 */
	helpers: TextFieldHelpers;
}

/**
 * Create a textfield
 *
 * @param config - TextField configuration
 * @returns TextField context
 */
export function createTextField(config: TextFieldConfig = {}): TextField {
	const {
		value: initialValue = '',
		placeholder = '',
		disabled = false,
		required = false,
		readonly = false,
		multiline = false,
		maxLength,
		minLength,
		pattern,
		type = 'text',
		autocomplete,
		label,
		describedBy,
		validate: customValidate,
		onChange,
		onFocus,
		onBlur,
		onSubmit,
		onDestroy,
	} = config;

	// Internal state
	let fieldElement: HTMLInputElement | HTMLTextAreaElement | null = null;

	// Reactive state (works in both Svelte and test environments)
	const internalState: TextFieldState = {
		value: initialValue,
		focused: false,
		touched: false,
		valid: true,
		error: null,
		length: initialValue.length,
		exceededLength: maxLength ? initialValue.length > maxLength : false,
		disabled,
		required,
		readonly,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof TextFieldState, value) {
			target[prop] = value as never;
			updateDOM();
			return true;
		},
	});

	function updateDOM() {
		if (fieldElement) {
			fieldElement.value = state.value;
			fieldElement.disabled = state.disabled;
			fieldElement.readOnly = state.readonly;
			if (state.required) {
				fieldElement.setAttribute('required', '');
			} else {
				fieldElement.removeAttribute('required');
			}
			if (state.error) {
				fieldElement.setAttribute('aria-invalid', 'true');
			} else {
				fieldElement.removeAttribute('aria-invalid');
			}
		}
	}

	/**
	 * Validate the value
	 */
	function validateValue(value: string): string | null {
		// Required validation
		if (required && !value.trim()) {
			return 'This field is required';
		}

		// Min length validation
		if (minLength && value.length < minLength && value.length > 0) {
			return `Minimum ${minLength} characters required`;
		}

		// Max length validation
		if (maxLength && value.length > maxLength) {
			return `Maximum ${maxLength} characters allowed`;
		}

		// Pattern validation
		if (pattern && value.length > 0) {
			const regex = new RegExp(pattern);
			if (!regex.test(value)) {
				return 'Invalid format';
			}
		}

		// Custom validation
		if (customValidate) {
			const customError = customValidate(value);
			if (customError) {
				return customError;
			}
		}

		return null;
	}

	/**
	 * Handle input change
	 */
	function handleInput(event: Event) {
		const target = event.target as HTMLInputElement | HTMLTextAreaElement;
		const newValue = target.value;

		state.value = newValue;
		state.length = newValue.length;
		state.exceededLength = maxLength ? newValue.length > maxLength : false;

		// Validate if touched
		if (state.touched) {
			const error = validateValue(newValue);
			state.error = error;
			state.valid = error === null;
		}

		onChange?.(newValue);
	}

	/**
	 * Handle focus
	 */
	function handleFocus() {
		state.focused = true;
		onFocus?.();
	}

	/**
	 * Handle blur
	 */
	function handleBlur() {
		state.focused = false;
		state.touched = true;

		// Validate on blur
		const error = validateValue(state.value);
		state.error = error;
		state.valid = error === null;

		onBlur?.();
	}

	/**
	 * Handle key down
	 */
	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' && !multiline) {
			event.preventDefault();
			onSubmit?.(state.value);
		}
	}

	/**
	 * Field action
	 */
	function field(node: HTMLInputElement | HTMLTextAreaElement) {
		fieldElement = node;

		// Set initial attributes
		if (placeholder) node.placeholder = placeholder;
		if (disabled) node.disabled = true;
		if (required) node.required = true;
		if (readonly) node.readOnly = true;
		if (maxLength) node.maxLength = maxLength;
		if (pattern && node instanceof HTMLInputElement) node.pattern = pattern;
		if (type && node instanceof HTMLInputElement) node.type = type;
		if (autocomplete && node instanceof HTMLInputElement) {
			// Type assertion needed because autocomplete property type is overly restrictive
			node.setAttribute('autocomplete', autocomplete);
		}
		if (label) node.setAttribute('aria-label', label);
		if (describedBy) node.setAttribute('aria-describedby', describedBy);

		// Set initial value
		node.value = state.value;

		// Add event listeners
		node.addEventListener('input', handleInput as EventListener);
		node.addEventListener('focus', handleFocus);
		node.addEventListener('blur', handleBlur);
		node.addEventListener('keydown', handleKeyDown as EventListener);

		return {
			destroy() {
				if (fieldElement) {
					node.removeEventListener('input', handleInput as EventListener);
					node.removeEventListener('focus', handleFocus);
					node.removeEventListener('blur', handleBlur);
					node.removeEventListener('keydown', handleKeyDown as EventListener);
					fieldElement = null;
				}
				onDestroy?.();
			},
		};
	}

	/**
	 * Set value helper
	 */
	function setValue(value: string) {
		state.value = value;
		state.length = value.length;
		state.exceededLength = maxLength ? value.length > maxLength : false;

		if (fieldElement) {
			fieldElement.value = value;
		}

		// Validate if touched
		if (state.touched) {
			const error = validateValue(value);
			state.error = error;
			state.valid = error === null;
		}
	}

	/**
	 * Clear helper
	 */
	function clear() {
		setValue('');
	}

	/**
	 * Validate helper
	 */
	function validate(): boolean {
		const error = validateValue(state.value);
		state.error = error;
		state.valid = error === null;
		state.touched = true;
		return state.valid;
	}

	/**
	 * Mark touched helper
	 */
	function markTouched() {
		state.touched = true;
	}

	/**
	 * Focus helper
	 */
	function focus() {
		fieldElement?.focus();
	}

	/**
	 * Blur helper
	 */
	function blur() {
		fieldElement?.blur();
	}

	return {
		state,
		actions: {
			field,
		},
		helpers: {
			setValue,
			clear,
			validate,
			markTouched,
			focus,
			blur,
		},
	};
}
