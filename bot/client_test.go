package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(handler http.HandlerFunc) (*CoreClient, *httptest.Server) {
	server := httptest.NewServer(handler)
	client := NewCoreClient(server.URL, "test-secret")
	return client, server
}

func TestCreateUserSendsCorrectRequest(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody UserUpsertRequest

	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := client.CreateUser(42, "free")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/users" {
		t.Errorf("path = %q, want /api/v1/users", gotPath)
	}
	if gotAuth != "Bearer test-secret" {
		t.Errorf("Authorization = %q, want Bearer test-secret", gotAuth)
	}
	if gotBody.TgID != 42 || gotBody.Tier != "free" {
		t.Errorf("body = %+v, want TgID=42 Tier=free", gotBody)
	}
}

func TestCreateUserReturnsErrorOnNonOKStatus(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()

	if err := client.CreateUser(1, "free"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApprovePaymentSendsCorrectRequest(t *testing.T) {
	var gotPath, gotMethod, gotAuth string

	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := client.ApprovePayment(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/payments/7/approve" {
		t.Errorf("path = %q, want /api/v1/payments/7/approve", gotPath)
	}
	if gotAuth != "Bearer test-secret" {
		t.Errorf("Authorization = %q, want Bearer test-secret", gotAuth)
	}
}

func TestGetUserDecodesResponse(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UserResponse{TgID: 42, Tier: "premium"})
	})
	defer server.Close()

	user, err := client.GetUser(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil || user.Tier != "premium" {
		t.Errorf("user = %+v, want Tier=premium", user)
	}
}

func TestGetUserReturnsNilOnNotFound(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	user, err := client.GetUser(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != nil {
		t.Errorf("user = %+v, want nil", user)
	}
}
