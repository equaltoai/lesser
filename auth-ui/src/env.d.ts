/**
 * Ambient declarations the auth UI relies on at type-check time.
 *
 * `astro check` builds its own program and already knows these; `svelte-check`
 * type-checks the .svelte files on their own and does not. Both read this file,
 * so `import.meta.env` and the injected wallet provider type identically under
 * either one.
 *
 * This deliberately declares the small surface the app uses instead of
 * referencing `astro/client` wholesale: that reference also retypes asset
 * imports (`*.svg` becomes `ImageMetadata`), which contradicts the vendored
 * greater-components' own `string` typing and would report errors in library
 * code this repo does not own.
 */

interface ImportMetaEnv {
  /** Astro's configured base path, e.g. "/auth/". */
  readonly BASE_URL: string;
  /** Local-dev override for the Lesser API origin; unset in production builds. */
  readonly PUBLIC_LESSER_API_ORIGIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * The EIP-1193 provider a browser wallet extension injects. Only the subset the
 * auth UI actually calls is declared; it is optional because no wallet is
 * required to use Lesser.
 */
interface EthereumProvider {
  isMetaMask?: boolean;
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
}

interface Window {
  ethereum?: EthereumProvider;
}
