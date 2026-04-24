package model

import "time"

// Rule status enumeration.
const (
	StatusPending = "pending"
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
	StatusFailed  = "failed"
)

// User role enumeration. PortPass splits concerns into two roles:
// admin owns the instance (user/preset management, unrestricted rule
// creation) and user is limited to rules whose (port, protocol) is
// whitelisted via PresetPort.UserAllowed and within MaxDurationSec.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Protocol enumeration.
const (
	ProtoTCP  = "tcp"
	ProtoUDP  = "udp"
	ProtoBoth = "both"
)

// Rule is a temporary firewall rule that opens a port group for a
// specific source IP until ExpireAt elapses. Port carries the lowest
// port of the group for backwards compatibility with earlier releases
// and index-based filtering; Ports carries the full canonical string
// ("80,443,8080-8090") that the driver layer consumes.
type Rule struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"index" json:"user_id"`
	SourceIP     string     `gorm:"index;size:64" json:"source_ip"`
	Port         int        `gorm:"index" json:"port"`
	Ports        string     `gorm:"size:256" json:"ports"`
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

// PresetPort is a reusable port entry. Beyond being the UI quick-button it
// doubles as the per-user port whitelist: non-admin users without a
// personal AllowedRange policy can open ports whose (preset with
// UserAllowed=true) covers the requested set, bounded by MaxDurationSec
// (0 means inherit global cap). Ports is the canonical port-group string.
type PresetPort struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	Name           string `gorm:"size:32" json:"name"`
	Port           int    `json:"port"`
	Ports          string `gorm:"size:256" json:"ports"`
	Protocol       string `gorm:"size:8" json:"protocol"`
	Sort           int    `gorm:"column:sort_order" json:"sort"`
	UserAllowed    bool   `gorm:"default:false" json:"user_allowed"`
	MaxDurationSec int    `gorm:"default:0" json:"max_duration_sec"`
}

// ProtectedPort declares a port group the operator has marked as in-use
// by the server's own services. Rules attempting to open any port inside
// a ProtectedPort are rejected for everyone, admin included. Use-case:
// prevent accidentally temporarily-opening ports like 22 (SSH) or 3306
// (MySQL) that should remain under the operator's controlled policy.
type ProtectedPort struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"size:32" json:"name"`
	Ports    string `gorm:"size:256" json:"ports"`
	Protocol string `gorm:"size:8" json:"protocol"`
	Note     string `gorm:"size:255" json:"note"`
}

// UserAllowedRange is an admin-issued per-user port-range override. When
// at least one row exists for a user the ensurePortPolicy chain uses
// these ranges instead of the preset.user_allowed default. A user with
// zero rows keeps the default behaviour (falls back to preset.user_allowed).
type UserAllowedRange struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	UserID         uint   `gorm:"index;not null" json:"user_id"`
	Name           string `gorm:"size:32" json:"name"`
	Ports          string `gorm:"size:256" json:"ports"`
	Protocol       string `gorm:"size:8" json:"protocol"`
	MaxDurationSec int    `gorm:"default:0" json:"max_duration_sec"`
	Note           string `gorm:"size:255" json:"note"`
}

// User is an account stored in the local SQLite database. The password is
// always persisted as a bcrypt hash; the plaintext is never stored nor
// returned via JSON. Role governs what the user may see and do.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:128;not null" json:"-"`
	Role         string    `gorm:"size:16;not null;default:user" json:"role"`
	Disabled     bool      `gorm:"default:false" json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
