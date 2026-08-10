/**
 * The product's own name, used wherever no operator-configured site name exists.
 *
 * This is a fork: upstream defaults to "Sub2API" in half a dozen places, and
 * every one of them is customer-visible (the login wordmark, the tab title, the
 * plaza nav). Keeping the fallback in one constant means renaming the product is
 * one edit rather than a grep, and a missed spot cannot ship the upstream name
 * on a screen a customer is looking at.
 *
 * An operator's configured `site_name` always wins over this -- their brand is
 * theirs, and this is only the floor.
 */
export const PRODUCT_NAME = 'Inferno'

/** Tab title suffix. Kept beside the name so the two never drift. */
export const PRODUCT_TAGLINE = 'AI API Gateway'
