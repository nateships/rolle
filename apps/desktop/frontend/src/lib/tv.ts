import { createTV } from "tailwind-variants";

export type { VariantProps } from "tailwind-variants";

/**
 * Variant classes for the UI primitives. Concatenation only: `cn` merges
 * Tailwind conflicts once at the call site, so tv does not need tailwind-merge.
 */
export const tv = createTV({ twMerge: false });
