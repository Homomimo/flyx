package model

const (
	// SubscriptionOwn is for the admin's own devices: owner credentials.
	SubscriptionOwn = "own"
	// SubscriptionShare is for someone else: own credentials, cut off when
	// disabled or expired.
	SubscriptionShare = "share"
)

// Subscription is one subscription link: which inbounds it draws from and
// how the node list is processed (see kernel.SubOptions). Flag columns
// default to 0: GORM replaces a zero value with the column default on
// create, so a default of 1 could never be saved as false.
type Subscription struct {
	ID    int64  `gorm:"primaryKey;autoIncrement"`
	Name  string `gorm:"type:varchar(100);not null"`
	Kind  string `gorm:"type:varchar(10);not null;default:'own'"`
	Token string `gorm:"type:varchar(64);not null;uniqueIndex"`
	// UUID / Password are the credentials of a share subscription.
	UUID     string `gorm:"column:uuid;type:varchar(36);not null;default:''"`
	Password string `gorm:"type:varchar(64);not null;default:''"`
	// SourceAll includes every enabled inbound (new ones too); otherwise
	// InboundIDs (comma separated) lists them.
	SourceAll  int    `gorm:"column:source_all;not null;default:0"`
	InboundIDs string `gorm:"column:inbound_ids;type:text;not null;default:''"`
	// Options is the JSON processing pipeline.
	Options string `gorm:"type:text;not null;default:''"`
	// ExpireTime is unix ms; 0 never expires.
	ExpireTime int64 `gorm:"column:expire_time;not null;default:0"`
	Enabled    int   `gorm:"not null;default:0"`
	// AppliedActive is the active state last pushed to the kernels, so the
	// expiry loop knows whose credentials still have to be removed.
	AppliedActive int    `gorm:"column:applied_active;not null;default:0"`
	Remark        string `gorm:"type:text;not null;default:''"`
	Inx           int    `gorm:"not null;default:0"`
	CreatedTime   int64  `gorm:"column:created_time;not null"`
	UpdatedTime   int64  `gorm:"column:updated_time;not null"`
}

func (Subscription) TableName() string { return "subscription" }
