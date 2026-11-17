package store

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"
)

// InitializeItems atomically inserts missing configuration keys without updating existing rows.
func (s *Store) InitializeItems(ctx context.Context, items []ConfigItem) error {
	if len(items) == 0 {
		return nil
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&items).Error; err != nil {
			return fmt.Errorf("initialize configuration defaults: %w", err)
		}
		return nil
	})
}
