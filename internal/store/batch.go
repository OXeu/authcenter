package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// GetItems reads selected keys within one project and environment.
func (s *Store) GetItems(ctx context.Context, projectID, envID uint, keys []string) ([]ConfigItem, error) {
	items := []ConfigItem{}
	if err := s.db.WithContext(ctx).Where("project_id = ? AND environment_id = ?", projectID, envID).Where(clause.IN{Column: "key", Values: keyValues(keys)}).Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("read configuration keys: %w", err)
	}
	return items, nil
}

func keyValues(keys []string) []any {
	values := make([]any, len(keys))
	for i, key := range keys {
		values[i] = key
	}
	return values
}

// PutItems atomically creates or updates all supplied configuration items.
func (s *Store) PutItems(ctx context.Context, items []ConfigItem) error {
	if len(items) == 0 {
		return nil
	}
	return s.Transaction(ctx, func(tx *Store) error {
		query := tx.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		conflict := clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}, {Name: "environment_id"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "description", "updated_by", "updated_at"}),
		}
		if err := query.Clauses(conflict).CreateInBatches(&items, 100).Error; err != nil {
			return fmt.Errorf("write configuration batch: %w", err)
		}
		return nil
	})
}
