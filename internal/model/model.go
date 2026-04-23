package model

import "time"

// Rule status enumeration.
const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusExpired  = "expired"
	StatusRevoked  = "revoked"
	StatusFailed   = "failed"
)

// Protocol enumeration.
const (
	ProtoTCP  = "tcp"
	ProtoUDP  = "udp"
	ProtoBoth = "both"
)

// Rule is a temporary firewall rule that opens a single port/range for a
// specific source IP until ExpireAt elapses.
type Rule struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	SourceIP     string     `gorm:"index;size:64" json:"source_ip"`
	Port         int        `gorm:"index" json:"port"`
	Protocol     string     `gorm:"size:8" json:"protocol"`
	Note         string     `gorm:"size:255" json:"note"`
	Status       string     `gorm:"index;size:16" json:"status"`
	ExpireAt     time.Time  `gorm:"index" json:"expire_at"`
	CreatedBy    string     `gorm:"size:64" json:"created_by"`
	CreatedIP    string     `gorm:"size:64" json:"created_ip"`
	CreatedAt    time.Time  `json:"created_at"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
	DriverName   string     `gorm:"size:16" json:"driver_name"`
	DriverRef    string     `gorm:"size:128" json:"driver_ref"`
	CommentTag   string     `gorm:"uniqueIndex;size:64" json:"comment_tag"`
}

// PresetPort is a reusable one-click port entry shown on the create-rule form.
type PresetPort struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"size:32" json:"name"`
	Port     int    `json:"port"`
	Protocol string `gorm:"size:8" json:"protocol"`
	Sort     int    `gorm:"column:sort_order" json:"sort"`
}

// Setting is a free-form key/value configuration row. Used for runtime-mutable
// preferences that are more convenient to edit in the UI than via env vars.
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditLog records every mutating action for compliance and forensics.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Action    string    `gorm:"index;size:32" json:"action"`
	RuleID    uint      `gorm:"index" json:"rule_id"`
	Actor     string    `gorm:"size:64" json:"actor"`
	ActorIP   string    `gorm:"size:64" json:"actor_ip"`
	Detail    string    `gorm:"type:text" json:"detail"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
