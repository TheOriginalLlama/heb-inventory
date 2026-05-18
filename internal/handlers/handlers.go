package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/TheOriginalLlama/heb-inventory/internal/store"
)

type API struct {
	Store  *store.Store
	Logger *slog.Logger
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", a.createItem)
	mux.HandleFunc("GET /items", a.listItems)
	mux.HandleFunc("GET /items/{sku}", a.getItem)
	mux.HandleFunc("PUT /items/{sku}", a.updateItem)
	mux.HandleFunc("POST /items/{sku}/adjust", a.adjustStock)
	mux.HandleFunc("GET /stores/{storeID}/stock", a.storeStock)
	return mux
}

type errorResp struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResp{Error: msg})
}

func (a *API) createItem(w http.ResponseWriter, r *http.Request) {
	var it store.Item
	if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if it.SKU == "" || it.Name == "" {
		writeErr(w, http.StatusBadRequest, "sku and name are required")
		return
	}
	created, err := a.Store.CreateItem(r.Context(), it)
	if err != nil {
		a.Logger.Error("create item failed", "err", err, "sku", it.SKU)
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) getItem(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")
	it, err := a.Store.GetItem(r.Context(), sku)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		a.Logger.Error("get item failed", "err", err, "sku", sku)
		writeErr(w, http.StatusInternalServerError, "get failed")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (a *API) listItems(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)
	if limit < 1 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	items, err := a.Store.ListItems(r.Context(), limit, offset)
	if err != nil {
		a.Logger.Error("list items failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) updateItem(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")
	var it store.Item
	if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	it.SKU = sku
	updated, err := a.Store.UpdateItem(r.Context(), it)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		a.Logger.Error("update item failed", "err", err, "sku", sku)
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type adjustReq struct {
	StoreID string `json:"store_id"`
	Delta   int64  `json:"delta"`
	Reason  string `json:"reason"`
}

func (a *API) adjustStock(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")
	var req adjustReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.StoreID == "" || req.Delta == 0 {
		writeErr(w, http.StatusBadRequest, "store_id and non-zero delta required")
		return
	}
	lvl, err := a.Store.AdjustStock(r.Context(), sku, req.StoreID, req.Delta, req.Reason)
	if err != nil {
		a.Logger.Error("adjust stock failed", "err", err, "sku", sku, "store_id", req.StoreID)
		writeErr(w, http.StatusInternalServerError, "adjust failed")
		return
	}
	writeJSON(w, http.StatusOK, lvl)
}

func (a *API) storeStock(w http.ResponseWriter, r *http.Request) {
	storeID := r.PathValue("storeID")
	levels, err := a.Store.StockForStore(r.Context(), storeID)
	if err != nil {
		a.Logger.Error("store stock failed", "err", err, "store_id", storeID)
		writeErr(w, http.StatusInternalServerError, "stock fetch failed")
		return
	}
	writeJSON(w, http.StatusOK, levels)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
