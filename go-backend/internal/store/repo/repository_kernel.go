package repo

import (
	"errors"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InboundCounterSample is one cumulative nft counter reported by an agent.
type InboundCounterSample struct {
	InboundID int64
	Protocol  string
	Direction string // "in" (client upload) or "out" (client download)
	Epoch     string
	Bytes     int64
}

func (r *Repository) ListNodeKernels(nodeID int64) ([]model.NodeKernel, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []model.NodeKernel
	q := r.db.Order("node_id ASC, id ASC")
	if nodeID > 0 {
		q = q.Where("node_id = ?", nodeID)
	}
	return rows, q.Find(&rows).Error
}

func (r *Repository) ListKernelNodeIDs() ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.NodeKernel{}).Distinct("node_id").Order("node_id ASC").Pluck("node_id", &ids).Error
	return ids, err
}

func (r *Repository) GetNodeKernel(id int64) (*model.NodeKernel, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var k model.NodeKernel
	err := r.db.Where("id = ?", id).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &k, err
}

func (r *Repository) GetNodeKernelByType(nodeID int64, kernelType string) (*model.NodeKernel, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var k model.NodeKernel
	err := r.db.Where("node_id = ? AND kernel_type = ?", nodeID, kernelType).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &k, err
}

func (r *Repository) CreateNodeKernel(k *model.NodeKernel) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Create(k).Error
}

// UpdateNodeKernelFields updates selected columns; zero values are written.
func (r *Repository) UpdateNodeKernelFields(id int64, fields map[string]interface{}) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.NodeKernel{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repository) DeleteNodeKernel(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var ids []int64
		if err := tx.Model(&model.NodeInbound{}).Where("kernel_id = ?", id).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := tx.Where("inbound_id IN ?", ids).Delete(&model.NodeInboundCounter{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("kernel_id = ?", id).Delete(&model.NodeInbound{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.NodeKernel{}).Error
	})
}

func deleteNodeKernelsTx(tx *gorm.DB, nodeID int64) error {
	var ids []int64
	if err := tx.Model(&model.NodeInbound{}).Where("node_id = ?", nodeID).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) > 0 {
		if err := tx.Where("inbound_id IN ?", ids).Delete(&model.NodeInboundCounter{}).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("node_id = ?", nodeID).Delete(&model.NodeInbound{}).Error; err != nil {
		return err
	}
	return tx.Where("node_id = ?", nodeID).Delete(&model.NodeKernel{}).Error
}

func (r *Repository) ListNodeInbounds() ([]model.NodeInbound, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []model.NodeInbound
	return rows, r.db.Order("inx ASC, id ASC").Find(&rows).Error
}

func (r *Repository) ListNodeInboundsByKernel(kernelID int64) ([]model.NodeInbound, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []model.NodeInbound
	return rows, r.db.Where("kernel_id = ?", kernelID).Order("id ASC").Find(&rows).Error
}

func (r *Repository) GetNodeInbound(id int64) (*model.NodeInbound, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var in model.NodeInbound
	err := r.db.Where("id = ?", id).First(&in).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &in, err
}

func (r *Repository) CreateNodeInbound(in *model.NodeInbound) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Create(in).Error
}

// SaveNodeInbound writes the editable columns; traffic is left untouched so a
// concurrent collection is not overwritten.
func (r *Repository) SaveNodeInbound(in *model.NodeInbound) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.NodeInbound{}).Where("id = ?", in.ID).Updates(map[string]interface{}{
		"name":         in.Name,
		"protocol":     in.Protocol,
		"listen":       in.Listen,
		"port":         in.Port,
		"transport":    in.Transport,
		"security":     in.Security,
		"settings":     in.Settings,
		"extra":        in.Extra,
		"share_host":   in.ShareHost,
		"remark":       in.Remark,
		"enabled":      in.Enabled,
		"updated_time": in.UpdatedTime,
	}).Error
}

func (r *Repository) DeleteNodeInbound(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("inbound_id = ?", id).Delete(&model.NodeInboundCounter{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.NodeInbound{}).Error
	})
}

func (r *Repository) ResetNodeInboundTraffic(id int64, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.NodeInbound{}).Where("id = ?", id).Updates(map[string]interface{}{
		"upload_bytes":   0,
		"download_bytes": 0,
		"updated_time":   now,
	}).Error
}

func listNodeInboundPortsTx(tx *gorm.DB, nodeID int64, excludeInboundID int64) ([]int, error) {
	var ports []int
	q := tx.Model(&model.NodeInbound{}).Where("node_id = ? AND port > 0", nodeID)
	if excludeInboundID > 0 {
		q = q.Where("id <> ?", excludeInboundID)
	}
	return ports, q.Pluck("port", &ports).Error
}

// NodePortOccupant names what already uses port on a node, or "" when free.
func (r *Repository) NodePortOccupant(nodeID int64, port int, excludeInboundID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	var count int64
	q := r.db.Model(&model.NodeInbound{}).Where("node_id = ? AND port = ?", nodeID, port)
	if excludeInboundID > 0 {
		q = q.Where("id <> ?", excludeInboundID)
	}
	if err := q.Count(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		return "其他节点服务", nil
	}
	if err := r.db.Model(&model.ForwardPort{}).Where("node_id = ? AND port = ?", nodeID, port).Count(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		return "转发", nil
	}
	if err := r.db.Model(&model.ChainTunnel{}).Where("node_id = ? AND port = ?", nodeID, port).Count(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		return "隧道", nil
	}
	return "", nil
}

// ApplyInboundCounterSamples converts cumulative counters into traffic deltas.
// A sample whose epoch differs from the stored one, or whose value went
// backwards, belongs to a fresh counter and counts from zero.
func (r *Repository) ApplyInboundCounterSamples(samples []InboundCounterSample, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if len(samples) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		ids := make([]int64, 0, len(samples))
		for _, s := range samples {
			ids = append(ids, s.InboundID)
		}
		var existing []int64
		if err := tx.Model(&model.NodeInbound{}).Where("id IN ?", ids).Pluck("id", &existing).Error; err != nil {
			return err
		}
		known := make(map[int64]struct{}, len(existing))
		for _, id := range existing {
			known[id] = struct{}{}
		}

		var states []model.NodeInboundCounter
		if err := tx.Where("inbound_id IN ?", ids).Find(&states).Error; err != nil {
			return err
		}
		type key struct {
			id        int64
			proto     string
			direction string
		}
		prev := make(map[key]model.NodeInboundCounter, len(states))
		for _, st := range states {
			prev[key{st.InboundID, st.Protocol, st.Direction}] = st
		}

		upload := make(map[int64]int64)
		download := make(map[int64]int64)
		for _, s := range samples {
			if _, ok := known[s.InboundID]; !ok || s.Bytes < 0 {
				continue
			}
			if s.Direction != "in" && s.Direction != "out" {
				continue
			}
			delta := s.Bytes
			if old, ok := prev[key{s.InboundID, s.Protocol, s.Direction}]; ok && old.Epoch == s.Epoch && s.Bytes >= old.Bytes {
				delta = s.Bytes - old.Bytes
			}
			if delta > 0 {
				if s.Direction == "in" {
					upload[s.InboundID] += delta
				} else {
					download[s.InboundID] += delta
				}
			}
			row := model.NodeInboundCounter{
				InboundID:   s.InboundID,
				Protocol:    s.Protocol,
				Direction:   s.Direction,
				Epoch:       s.Epoch,
				Bytes:       s.Bytes,
				UpdatedTime: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "inbound_id"}, {Name: "protocol"}, {Name: "direction"}},
				DoUpdates: clause.AssignmentColumns([]string{"epoch", "bytes", "updated_time"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		for id := range known {
			up, down := upload[id], download[id]
			if up == 0 && down == 0 {
				continue
			}
			if err := tx.Model(&model.NodeInbound{}).Where("id = ?", id).Updates(map[string]interface{}{
				"upload_bytes":   gorm.Expr("upload_bytes + ?", up),
				"download_bytes": gorm.Expr("download_bytes + ?", down),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
