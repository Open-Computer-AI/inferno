//go:build unit

package admin

import (
	"encoding/json"
	"testing"
)

func TestValidateOpsAlertRulePayloadAcceptsLatencyPercentiles(t *testing.T) {
	t.Parallel()

	for _, metricType := range []string{"p95_latency_ms", "p99_latency_ms"} {
		t.Run(metricType, func(t *testing.T) {
			t.Parallel()

			raw := map[string]json.RawMessage{
				"name":        json.RawMessage(`"Latency alert"`),
				"metric_type": json.RawMessage(`"` + metricType + `"`),
				"operator":    json.RawMessage(`">"`),
				"threshold":   json.RawMessage(`2000`),
			}
			if _, err := validateOpsAlertRulePayload(raw); err != nil {
				t.Fatalf("%s should be accepted as a latency metric: %v", metricType, err)
			}
		})
	}
}
