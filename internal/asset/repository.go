package asset

import (
	"context"
	"database/sql"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r PostgresRepository) ListAssets(ctx context.Context, userID string) ([]Asset, error) {
	rows, err := r.DB.QueryContext(ctx, assetSelectQuery("assets")+`
WHERE user_id = $1
ORDER BY
	CASE asset_type WHEN 'Liquid Asset' THEN 1 WHEN 'Fixed Asset' THEN 2 ELSE 3 END,
	category ASC,
	name ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assets := []Asset{}
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func (r PostgresRepository) CreateAsset(ctx context.Context, userID string, req SaveAssetRequest) (Asset, error) {
	return scanAssetRow(r.DB.QueryRowContext(ctx, `
WITH inserted AS (
	INSERT INTO assets (user_id, asset_type, category, name, quantity, unit, purchase_price, current_price, current_value, note, acquired_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	RETURNING *
)
`+assetSelectQuery("inserted")+`
`, userID, req.AssetType, req.Category, req.Name, req.Quantity, req.Unit, req.PurchasePrice, req.CurrentPrice, req.CurrentValue, req.Note, req.AcquiredAt))
}

func (r PostgresRepository) UpdateAsset(ctx context.Context, userID, assetID string, req SaveAssetRequest) (Asset, error) {
	return scanAssetRow(r.DB.QueryRowContext(ctx, `
WITH updated AS (
	UPDATE assets
	SET asset_type = $3,
		category = $4,
		name = $5,
		quantity = $6,
		unit = $7,
		purchase_price = $8,
		current_price = $9,
		current_value = $10,
		note = $11,
		acquired_at = $12,
		updated_at = NOW()
	WHERE user_id = $1 AND id = $2
	RETURNING *
)
`+assetSelectQuery("updated")+`
`, userID, assetID, req.AssetType, req.Category, req.Name, req.Quantity, req.Unit, req.PurchasePrice, req.CurrentPrice, req.CurrentValue, req.Note, req.AcquiredAt))
}

func (r PostgresRepository) DeleteAsset(ctx context.Context, userID, assetID string) error {
	result, err := r.DB.ExecContext(ctx, `
DELETE FROM assets
WHERE user_id = $1 AND id = $2
`, userID, assetID)
	if err != nil {
		return err
	}
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func assetSelectQuery(source string) string {
	return `
SELECT id::text, user_id::text, asset_type, category, name, quantity, unit, purchase_price, current_price, current_value,
	(quantity * purchase_price) AS cost_basis,
	(current_value - (quantity * purchase_price)) AS gain_loss,
	CASE WHEN (quantity * purchase_price) > 0 THEN ((current_value - (quantity * purchase_price)) / (quantity * purchase_price)) * 100 ELSE 0 END AS gain_loss_pct,
	note, acquired_at::text, created_at, updated_at
FROM ` + source + `
`
}

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAssetRow(row assetScanner) (Asset, error) {
	return scanAsset(row)
}

func scanAsset(scanner assetScanner) (Asset, error) {
	var asset Asset
	var acquiredAt sql.NullString
	err := scanner.Scan(
		&asset.ID,
		&asset.UserID,
		&asset.AssetType,
		&asset.Category,
		&asset.Name,
		&asset.Quantity,
		&asset.Unit,
		&asset.PurchasePrice,
		&asset.CurrentPrice,
		&asset.CurrentValue,
		&asset.CostBasis,
		&asset.GainLoss,
		&asset.GainLossPct,
		&asset.Note,
		&acquiredAt,
		&asset.CreatedAt,
		&asset.UpdatedAt,
	)
	if err != nil {
		return Asset{}, err
	}
	if acquiredAt.Valid {
		asset.AcquiredAt = &acquiredAt.String
	}
	return asset, nil
}
