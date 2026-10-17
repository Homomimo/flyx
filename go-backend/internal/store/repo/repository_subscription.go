package repo

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
)

// Legacy settings (3.0.10) that became the default subscription.
const (
	legacySubTokenKey = "node_sub_token"
	legacySubRename   = "sub_rename_rules"
	legacySubPrefix   = "sub_name_prefix"
	legacySubSort     = "sub_sort"
	legacySubRegions  = "sub_region_groups"
)

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// migrateSubscriptions makes sure there is at least one subscription. On
// upgrade the old single subscription (token and naming settings) becomes
// the default one and shared users become share subscriptions, keeping
// their links and credentials.
func migrateSubscriptions(db *gorm.DB, now int64) error {
	var count int64
	if err := db.Model(&model.Subscription{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		cfg := map[string]string{}
		var rows []model.ViteConfig
		if err := tx.Where("name IN ?", []string{legacySubTokenKey, legacySubRename, legacySubPrefix, legacySubSort, legacySubRegions}).
			Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			cfg[r.Name] = r.Value
		}
		opts, _ := json.Marshal(map[string]interface{}{
			"rename":       cfg[legacySubRename],
			"prefix":       cfg[legacySubPrefix],
			"sort":         cfg[legacySubSort],
			"regionGroups": cfg[legacySubRegions] == "true",
		})
		token := cfg[legacySubTokenKey]
		if len(token) < 32 {
			token = randomToken()
		}
		def := model.Subscription{
			Name: "我的订阅", Kind: model.SubscriptionOwn, Token: token, SourceAll: 1,
			Options: string(opts), Enabled: 1, CreatedTime: now, UpdatedTime: now,
		}
		if err := tx.Create(&def).Error; err != nil {
			return err
		}

		var users []model.SubUser
		if err := tx.Order("id ASC").Find(&users).Error; err != nil {
			return err
		}
		for i, u := range users {
			sub := model.Subscription{
				Name: u.Name, Kind: model.SubscriptionShare, Token: u.Token, UUID: u.UUID, Password: u.Password,
				SourceAll: 0, InboundIDs: u.InboundIDs, Options: string(opts), ExpireTime: u.ExpireTime,
				Enabled: u.Enabled, AppliedActive: u.AppliedActive, Remark: u.Remark, Inx: i + 1,
				CreatedTime: u.CreatedTime, UpdatedTime: now,
			}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("1 = 1").Delete(&model.SubUser{}).Error; err != nil {
			return err
		}
		return tx.Where("name IN ?", []string{legacySubTokenKey, legacySubRename, legacySubPrefix, legacySubSort, legacySubRegions}).
			Delete(&model.ViteConfig{}).Error
	})
}

func (r *Repository) ListSubscriptions() ([]model.Subscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var out []model.Subscription
	err := r.db.Order("inx ASC, id ASC").Find(&out).Error
	return out, err
}

func (r *Repository) GetSubscription(id int64) (*model.Subscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var s model.Subscription
	res := r.db.Where("id = ?", id).Limit(1).Find(&s)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, res.Error
	}
	return &s, nil
}

func (r *Repository) GetSubscriptionByToken(token string) (*model.Subscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var s model.Subscription
	res := r.db.Where("token = ?", token).Limit(1).Find(&s)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, res.Error
	}
	return &s, nil
}

// DefaultSubscription is the first own subscription (the one the old
// single-subscription API refers to), created if none exists.
func (r *Repository) DefaultSubscription(now int64) (*model.Subscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var s model.Subscription
	res := r.db.Where("kind = ?", model.SubscriptionOwn).Order("inx ASC, id ASC").Limit(1).Find(&s)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected > 0 {
		return &s, nil
	}
	s = model.Subscription{Name: "我的订阅", Kind: model.SubscriptionOwn, Token: randomToken(), SourceAll: 1,
		Enabled: 1, CreatedTime: now, UpdatedTime: now}
	return &s, r.db.Create(&s).Error
}

func (r *Repository) SaveSubscription(s *model.Subscription) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Save(s).Error
}

func (r *Repository) DeleteSubscription(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("id = ?", id).Delete(&model.Subscription{}).Error
}

func (r *Repository) UpdateSubscriptionApplied(id int64, active int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Subscription{}).Where("id = ?", id).Update("applied_active", active).Error
}
