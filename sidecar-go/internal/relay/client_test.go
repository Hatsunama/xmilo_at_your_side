package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xmilo/sidecar-go/shared/contracts"
)

func TestTurnFailsFastWithoutJWT(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("relay should not be called when JWT is absent")
	}))
	defer server.Close()

	client := New(server.URL, func() (string, error) {
		return "", nil
	})

	_, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_1"})
	if err == nil {
		t.Fatal("expected entitlement_lost error")
	}
	if !strings.Contains(err.Error(), "entitlement_lost") {
		t.Fatalf("expected entitlement_lost error, got %v", err)
	}
	if called {
		t.Fatal("relay endpoint was called unexpectedly")
	}
}
