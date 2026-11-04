package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientSendsKeyAndDecodesData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ct_mcp_test" || r.Header.Get("User-Agent") == "" {
			t.Errorf("missing auth or user agent: %v", r.Header)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":7,"version":"abc"}}`))
	}))
	defer srv.Close()

	var out struct {
		ID      int    `json:"id"`
		Version string `json:"version"`
	}
	status, err := NewClient(srv.URL+"/", "ct_mcp_test", "test").Do(context.Background(), "PUT", appPath("my app", "heartbeats"), map[string]any{"name": "x"}, &out)
	if err != nil || status != http.StatusCreated || out.ID != 7 || out.Version != "abc" {
		t.Fatalf("status=%d err=%v out=%+v", status, err, out)
	}
}

func TestClientParsesErrorsAndConflicts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict","message":"This record changed since you read it. Read it again."}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "k", "test").Do(context.Background(), "PUT", "/x", map[string]any{}, nil)
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsConflict() || apiConflictDiag(err) == "" {
		t.Fatalf("expected a stale-version conflict, got %#v", err)
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	var out map[string]any
	if _, err := NewClient(srv.URL, "k", "test").Do(context.Background(), "GET", "/x", nil, &out); err != nil || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestReconcileTagsKeepsExplicitEmptySet(t *testing.T) {
	explicitEmpty := types.SetValueMust(types.StringType, []attr.Value{})
	if got := reconcileTags(explicitEmpty, nil); got.IsNull() {
		t.Fatal("tags = [] must stay an empty set, not become null")
	}
	if got := reconcileTags(types.SetNull(types.StringType), nil); !got.IsNull() {
		t.Fatal("unset tags must stay null")
	}
	if got := reconcileTags(types.SetNull(types.StringType), []string{"a"}); len(got.Elements()) != 1 {
		t.Fatal("tags from the API must be kept")
	}
}
