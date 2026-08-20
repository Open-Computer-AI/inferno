package service

// ProductName is this product's own name, used wherever no operator-configured
// site name exists. It is the backend twin of inferno-frontend's
// src/config/brand.ts PRODUCT_NAME, and the two must agree.
//
// Upstream (Wei-Shaw/sub2api) hardcodes "Sub2API" as the site_name default in
// three places. That default is customer-visible on the very first screen, and
// it silently defeated the frontend rebrand: the login view resolves
// `settings.site_name || PRODUCT_NAME`, and a non-empty backend default means
// the `||` fallback can never fire. The frontend was renamed on the side that
// always loses, so every fresh deploy still greeted users with "Sub2API".
//
// Keeping it as one constant means renaming the product is one edit here plus
// one in brand.ts, rather than a grep across defaults that only show up on a
// database with no settings row yet -- i.e. exactly the state a new customer's
// first login is in.
const ProductName = "Inferno"
