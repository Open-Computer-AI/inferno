package payment

// IsRazorpaySubscriptionLifecycleEvent reports whether a Razorpay webhook
// changes the state of a recurring subscription rather than reporting a
// payment attempt. These events are handled separately from payment
// fulfillment because they do not contain a payable order amount.
func IsRazorpaySubscriptionLifecycleEvent(event string) bool {
	switch event {
	case "subscription.authenticated",
		"subscription.activated",
		"subscription.pending",
		"subscription.halted",
		"subscription.cancelled",
		"subscription.completed",
		"subscription.paused",
		"subscription.resumed":
		return true
	default:
		return false
	}
}
