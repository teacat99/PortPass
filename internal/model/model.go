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

	// Expiry-notification fields. The lead time is snapshotted at rule
	// creation time so a later change to the global default does not
	// retroactively affect existing rules. The Sent* timestamps are
	// kept per-channel rather than as a single column because the
	// global "both" channel selector wants browser + ntfy to be
	// independent: a successful ntfy push must not silence the browser
	// pop-up scheduled for the same rule and vice versa. Both are
	// cleared on Extend so the next imminent expiry triggers another
	// round of notifications.
	NotifyEnabled       bool       `gorm:"default:false" json:"notify_enabled"`
	NotifyLeadSeconds   int        `gorm:"default:0" json:"notify_lead_seconds"`
	NotifySentBrowserAt *time.Time `json:"notify_sent_browser_at,omitempty"`
	NotifySentNtfyAt    *time.Time `json:"notify_sent_ntfy_at,omitempty"`

	// CleanupOnExpire flips on conntrack flushing for this rule when it
	// is removed from the firewall (auto expiry, manual revoke, or
	// reconcile-driven cleanup of an overdue rule). Filtering is by the
	// exact (source_ip, port, protocol) tuple so other firewall rules
	// covering different IPs or ports remain untouched. Stored
	// per-rule rather than as a global flag so different rules can
	// have different "kick the user out" semantics; the runtime
	// setting `cleanup_on_expire_default` only seeds the form value
	// at creation time and never affects rules already in the DB.
	CleanupOnExpire bool `gorm:"default:false" json:"cleanup_on_expire"`
	// LastCleanupCount records the number of conntrack entries deleted
	// the last time cleanup was triggered for this rule, so the UI can
	// surface "已断开 N 条旧连接" without re-querying the kernel. Zero
	// means either cleanup was disabled or it ran but found nothing.
	LastCleanupCount int `gorm:"default:0" json:"last_cleanup_count"`
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
	// CategoryID points to a PresetCategory row. nil means "auto-detect"
	// using the frontend heuristic (categorize by name/port). Manual
	// selection persists here so the UI can group presets without
	// re-running heuristics on every load.
	CategoryID *uint `gorm:"index" json:"category_id,omitempty"`
}

// PresetCategory groups preset ports for display purposes. Six built-in
// rows (remote/web/db/mq/game/misc) are seeded on first boot with
// Builtin=true; the UI prevents deletion of those rows but allows
// re-labelling and icon overrides. Operators may add their own custom
// categories beyond the built-in set, identified by Builtin=false and a
// blank Key.
type PresetCategory struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Key matches the heuristic slug used by the frontend categorize()
	// function: remote/web/db/mq/game/misc for built-ins; empty string
	// for user-defined entries.
	Key string `gorm:"size:32;index" json:"key"`
	// Label is the user-visible name. Empty for built-ins (the frontend
	// then falls back to its i18n string keyed by Key); a non-empty
	// value overrides the i18n string.
	Label string `gorm:"size:64" json:"label"`
	// Icon holds either an emoji glyph or an http(s):// image URL. The
	// frontend detects the kind by URL prefix and renders rounded
	// images for the URL case.
	Icon      string    `gorm:"size:255" json:"icon"`
	Sort      int       `gorm:"column:sort_order" json:"sort"`
	Builtin   bool      `gorm:"default:false" json:"builtin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// LoginAttempt records every authentication attempt (success or failure)
// so we can rate-limit attackers, detect brute force, and give real users
// visibility into activity on their account. Separate from AuditLog because
// we keep a tighter retention window (matching LoginFailWindow*) and the
// hot-path cardinality is very different.
type LoginAttempt struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"index;size:64" json:"username"`
	ClientIP  string    `gorm:"index;size:64" json:"client_ip"`
	Success   bool      `gorm:"index" json:"success"`
	Reason    string    `gorm:"size:64" json:"reason"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
