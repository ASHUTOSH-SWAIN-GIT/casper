//go:build integration

package integration

import "testing"

// Placeholder so `go test -tags=integration ./test/integration/...` has
// something to compile and run before the real interpreter+AWS test
// lands. Replace with real RDS resize end-to-end once the AWS client
// implementation exists.
func TestIntegrationPlaceholder(t *testing.T) {
	t.Log("integration scaffold ok")
}
