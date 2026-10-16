package repo

import (
	"errors"

	"go-backend/internal/store/model"
)

func (r *Repository) ListSubUsers() ([]model.SubUser, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var out []model.SubUser
	err := r.db.Order("id ASC").Find(&out).Error
	return out, err
}

func (r *Repository) GetSubUser(id int64) (*model.SubUser, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var u model.SubUser
	res := r.db.Where("id = ?", id).Limit(1).Find(&u)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, res.Error
	}
	return &u, nil
}

func (r *Repository) GetSubUserByToken(token string) (*model.SubUser, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var u model.SubUser
	res := r.db.Where("token = ?", token).Limit(1).Find(&u)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, res.Error
	}
	return &u, nil
}

func (r *Repository) SaveSubUser(u *model.SubUser) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Save(u).Error
}

func (r *Repository) DeleteSubUser(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("id = ?", id).Delete(&model.SubUser{}).Error
}

func (r *Repository) UpdateSubUserApplied(id int64, active int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.SubUser{}).Where("id = ?", id).Update("applied_active", active).Error
}
