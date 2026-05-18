package store

import (
	"context"
	"time"
)

type Item struct {
	SKU            string    `json:"sku"`
	Name           string    `json:"name"`
	Department     string    `json:"department"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type StockLevel struct {
	SKU       string    `json:"sku"`
	StoreID   string    `json:"store_id"`
	Quantity  int64     `json:"quantity"`
	UpdatedAt time.Time `json:"updated_at"`
}

type StockMovement struct {
	ID        int64     `json:"id"`
	SKU       string    `json:"sku"`
	StoreID   string    `json:"store_id"`
	Delta     int64     `json:"delta"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateItem(ctx context.Context, it Item) (Item, error) {
	const q = `
		INSERT INTO items (sku, name, department, unit_price_cents)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at`
	err := s.pool.QueryRow(ctx, q, it.SKU, it.Name, it.Department, it.UnitPriceCents).
		Scan(&it.CreatedAt, &it.UpdatedAt)
	return it, mapErr(err)
}

func (s *Store) GetItem(ctx context.Context, sku string) (Item, error) {
	const q = `
		SELECT sku, name, department, unit_price_cents, created_at, updated_at
		FROM items WHERE sku = $1`
	var it Item
	err := s.pool.QueryRow(ctx, q, sku).Scan(
		&it.SKU, &it.Name, &it.Department, &it.UnitPriceCents, &it.CreatedAt, &it.UpdatedAt)
	return it, mapErr(err)
}

func (s *Store) ListItems(ctx context.Context, limit, offset int) ([]Item, error) {
	const q = `
		SELECT sku, name, department, unit_price_cents, created_at, updated_at
		FROM items
		ORDER BY sku
		LIMIT $1 OFFSET $2`
	rows, err := s.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Item, 0)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.SKU, &it.Name, &it.Department, &it.UnitPriceCents, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) UpdateItem(ctx context.Context, it Item) (Item, error) {
	const q = `
		UPDATE items
		SET name = $2, department = $3, unit_price_cents = $4, updated_at = now()
		WHERE sku = $1
		RETURNING created_at, updated_at`
	err := s.pool.QueryRow(ctx, q, it.SKU, it.Name, it.Department, it.UnitPriceCents).
		Scan(&it.CreatedAt, &it.UpdatedAt)
	return it, mapErr(err)
}

// AdjustStock applies a delta to stock for (sku, store_id) and records a movement.
// Runs in a transaction to keep the level and movement log consistent.
func (s *Store) AdjustStock(ctx context.Context, sku, storeID string, delta int64, reason string) (StockLevel, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return StockLevel{}, err
	}
	defer tx.Rollback(ctx)

	const upsert = `
		INSERT INTO stock (sku, store_id, quantity, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (sku, store_id) DO UPDATE
		SET quantity = stock.quantity + EXCLUDED.quantity, updated_at = now()
		RETURNING sku, store_id, quantity, updated_at`
	var lvl StockLevel
	if err := tx.QueryRow(ctx, upsert, sku, storeID, delta).
		Scan(&lvl.SKU, &lvl.StoreID, &lvl.Quantity, &lvl.UpdatedAt); err != nil {
		return StockLevel{}, mapErr(err)
	}

	const ins = `
		INSERT INTO stock_movements (sku, store_id, delta, reason)
		VALUES ($1, $2, $3, $4)`
	if _, err := tx.Exec(ctx, ins, sku, storeID, delta, reason); err != nil {
		return StockLevel{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return StockLevel{}, err
	}
	return lvl, nil
}

func (s *Store) StockForStore(ctx context.Context, storeID string) ([]StockLevel, error) {
	const q = `
		SELECT sku, store_id, quantity, updated_at
		FROM stock WHERE store_id = $1
		ORDER BY sku`
	rows, err := s.pool.Query(ctx, q, storeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]StockLevel, 0)
	for rows.Next() {
		var lvl StockLevel
		if err := rows.Scan(&lvl.SKU, &lvl.StoreID, &lvl.Quantity, &lvl.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, lvl)
	}
	return out, rows.Err()
}
