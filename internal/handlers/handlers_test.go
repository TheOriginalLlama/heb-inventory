package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouting_MethodNotAllowed(t *testing.T) {
	a := &API{}
	srv := httptest.NewServer(a.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/items", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestCreateItem_InvalidJSON(t *testing.T) {
	a := &API{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader("not json"))
	a.createItem(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateItem_MissingFields(t *testing.T) {
	a := &API{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"sku":""}`))
	a.createItem(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
