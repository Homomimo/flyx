package model

// NodeKernel is a proxy kernel (Xray, sing-box, ...) deployed on a node.
type NodeKernel struct {
	ID               int64  `gorm:"primaryKey;autoIncrement"`
	NodeID           int64  `gorm:"column:node_id;not null;uniqueIndex:idx_node_kernel_node_type"`
	KernelType       string `gorm:"column:kernel_type;type:varchar(20);not null;uniqueIndex:idx_node_kernel_node_type"`
	Version          string `gorm:"type:varchar(50);not null;default:'latest'"`
	InstalledVersion string `gorm:"column:installed_version;type:varchar(100);not null;default:''"`
	Status           string `gorm:"type:varchar(20);not null;default:'pending'"`
	LastError        string `gorm:"column:last_error;type:text;not null;default:''"`
	ConfigHash       string `gorm:"column:config_hash;type:varchar(64);not null;default:''"`
	AppliedHash      string `gorm:"column:applied_hash;type:varchar(64);not null;default:''"`
	Enabled          int    `gorm:"not null;default:1"`
	CreatedTime      int64  `gorm:"column:created_time;not null"`
	UpdatedTime      int64  `gorm:"column:updated_time;not null"`
}

func (NodeKernel) TableName() string { return "node_kernel" }

// NodeInbound is one listening proxy service served by a NodeKernel. The
// protocol specific parts are stored as JSON text (see internal/kernel).
type NodeInbound struct {
	ID            int64  `gorm:"primaryKey;autoIncrement"`
	KernelID      int64  `gorm:"column:kernel_id;not null;index"`
	NodeID        int64  `gorm:"column:node_id;not null;index"`
	Name          string `gorm:"type:varchar(100);not null"`
	Protocol      string `gorm:"type:varchar(30);not null"`
	Listen        string `gorm:"type:varchar(100);not null;default:'::'"`
	Port          int    `gorm:"not null"`
	Transport     string `gorm:"type:text;not null;default:''"`
	Security      string `gorm:"type:text;not null;default:''"`
	Settings      string `gorm:"type:text;not null;default:''"`
	Extra         string `gorm:"type:text;not null;default:''"`
	ShareHost     string `gorm:"column:share_host;type:varchar(255);not null;default:''"`
	Remark        string `gorm:"type:text;not null;default:''"`
	Enabled       int    `gorm:"not null;default:1"`
	UploadBytes   int64  `gorm:"column:upload_bytes;not null;default:0"`
	DownloadBytes int64  `gorm:"column:download_bytes;not null;default:0"`
	Inx           int    `gorm:"not null;default:0"`
	CreatedTime   int64  `gorm:"column:created_time;not null"`
	UpdatedTime   int64  `gorm:"column:updated_time;not null"`
}

func (NodeInbound) TableName() string { return "node_inbound" }

// NodeInboundCounter remembers the last cumulative nft counter value seen for
// an inbound so traffic can be accumulated as deltas. Epoch changes whenever
// the agent rebuilds its counter table, which resets the counters.
type NodeInboundCounter struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	InboundID   int64  `gorm:"column:inbound_id;not null;uniqueIndex:idx_node_inbound_counter_key"`
	Protocol    string `gorm:"type:varchar(10);not null;uniqueIndex:idx_node_inbound_counter_key"`
	Direction   string `gorm:"type:varchar(10);not null;uniqueIndex:idx_node_inbound_counter_key"`
	Epoch       string `gorm:"type:varchar(64);not null;default:''"`
	Bytes       int64  `gorm:"not null;default:0"`
	UpdatedTime int64  `gorm:"column:updated_time;not null"`
}

func (NodeInboundCounter) TableName() string { return "node_inbound_counter" }
