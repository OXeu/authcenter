package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// InitializeItems atomically inserts missing configuration keys without updating existing rows.
func (s *Store) InitializeItems(ctx context.Context, items []ConfigItem) error {
	if len(items) == 0 {
		return nil
	}
	return s.Transaction(ctx, func(tx *Store) error {
		query := tx.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		if err := query.Clauses(clause.OnConflict{DoNothing: true}).Create(&items).Error; err != nil {
			return fmt.Errorf("initialize configuration defaults: %w", err)
		}
		return nil
	})
}
