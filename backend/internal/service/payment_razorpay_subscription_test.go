package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertRazorpaySubscriptionStatusNoteReplacesPreviousStatus(t *testing.T) {
	got := upsertRazorpaySubscriptionStatusNote(
		"paid by checkout\nRazorpay subscription status: pending (subscription.pending, event evt_old)",
		"Razorpay subscription status: cancelled (subscription.cancelled, event evt_new)",
	)

	require.Equal(t, "paid by checkout\nRazorpay subscription status: cancelled (subscription.cancelled, event evt_new)", got)
}

func TestUpsertRazorpaySubscriptionStatusNotePreservesExistingNotes(t *testing.T) {
	require.Equal(t,
		"Razorpay subscription status: halted (subscription.halted, event evt_1)",
		upsertRazorpaySubscriptionStatusNote("", "Razorpay subscription status: halted (subscription.halted, event evt_1)"),
	)
	require.Equal(t,
		"manual note\nRazorpay subscription status: active (subscription.activated, event evt_2)",
		upsertRazorpaySubscriptionStatusNote("manual note", "Razorpay subscription status: active (subscription.activated, event evt_2)"),
	)
}
