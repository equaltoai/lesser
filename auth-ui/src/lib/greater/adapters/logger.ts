import type { TransportLogger } from './types.js';

const noop = (): void => undefined;

const baseLogger: Required<TransportLogger> = {
	debug: noop,
	info: noop,
	warn: noop,
	error: noop,
};

export const defaultLogger = baseLogger;

export function resolveLogger(logger?: TransportLogger): Required<TransportLogger> {
	if (!logger) {
		return baseLogger;
	}

	return {
		debug: logger.debug ?? baseLogger.debug,
		info: logger.info ?? baseLogger.info,
		warn: logger.warn ?? baseLogger.warn,
		error: logger.error ?? baseLogger.error,
	};
}

export function createConsoleLogger(namespace: string): Required<TransportLogger> {
	const globalConsole = globalThis.console;
	if (!globalConsole) {
		return baseLogger;
	}

	const formatMessage = (level: string, message: string): string =>
		`[${namespace}] ${level.toUpperCase()}: ${message}`;

	return {
		debug: (message, context) => {
			const log = globalConsole.debug ?? globalConsole.log;
			if (log) {
				log(formatMessage('debug', message), context);
			}
		},
		info: (message, context) => {
			const log = globalConsole.info ?? globalConsole.log;
			if (log) {
				log(formatMessage('info', message), context);
			}
		},
		warn: (message, context) => {
			if (globalConsole.warn) {
				globalConsole.warn(formatMessage('warn', message), context);
			}
		},
		error: (message, error) => {
			if (globalConsole.error) {
				globalConsole.error(formatMessage('error', message), error);
			}
		},
	};
}
