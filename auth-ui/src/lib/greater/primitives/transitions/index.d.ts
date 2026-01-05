/**
 * Greater Components Svelte Transitions
 *
 * Pre-built transition functions for common animation patterns.
 * All transitions respect prefers-reduced-motion and use design token timing.
 *
 * @example
 * ```svelte
 * <script>
 *   import { fadeUp, scaleIn } from 'src/lib/greater/primitives/transitions';
 * </script>
 *
 * <div transition:fadeUp={{ duration: 300 }}>
 *   Animated content
 * </div>
 * ```
 */
export { fadeUp, type FadeUpParams } from './fadeUp.js';
export { fadeDown, type FadeDownParams } from './fadeDown.js';
export { slideIn, type SlideInParams } from './slideIn.js';
export { scaleIn, type ScaleInParams } from './scaleIn.js';
//# sourceMappingURL=index.d.ts.map
