package repo

import (
	"errors"

	"go-backend/internal/store/model"
)

func (r *Repository) ListMonitorNodes() ([]model.Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var nodes []model.Node
	err := r.db.Select("id", "inx", "name", "status", "version", "updated_time").
		Where("is_remote = ?", 0).
		Order("inx ASC, id ASC").
		Find(&nodes).Error
	return nodes, err
}

// ListNodesWithExpiry returns local nodes that have an expiry time set.
func (r *Repository) ListNodesWithExpiry() ([]model.Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var nodes []model.Node
	err := r.db.Select("id", "name", "expiry_time", "renewal_cycle", "expiry_reminder_dismissed").
		Where("is_remote = ? AND expiry_time IS NOT NULL AND expiry_time > 0", 0).
		Order("id ASC").
		Find(&nodes).Error
	return nodes, err
}
