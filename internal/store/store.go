package store

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/teacat99/PortPass/internal/model"
)

// DefaultAdminUsername is the seed admin account username used when the
// users table is empty and no explicit PORTPASS_ADMIN_USERNAME is provided.
const DefaultAdminUsername = "admin"

// DefaultAdminPassword is the fallback password seeded on first boot when
// PORTPASS_ADMIN_PASSWORD is not provided. It exists purely for out-of-box
// convenience; operators are expected to change it via the UI immediately.
const DefaultAdminPassword = "passwd"

// Store is a thin GORM wrapper that exposes intent-revealing helpers to the
// rest of the codebase instead of leaking *gorm.DB everywhere.
type Store struct {
	db *gorm.DB
}

// New opens (or creates) a SQLite database at path and runs migrations.
func New(path string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1) // SQLite writers are serialised

	if err := db.AutoMigrate(
		&model.Rule{},
		&model.PresetPort{},
		&model.Setting{},
		&model.AuditLog{},
		&model.User{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// SeedAdminIfEmpty ensures there is at least one administrator row in the
// users table. When the table is empty it inserts one (username taken from
// preferredUsername or "admin"; password from preferredPassword or the
// hard-coded "passwd" fallback). The function also backfills any legacy
// rules that lack UserID so they are attributed to the seeded admin.
//
// This must only run during bootstrap; it returns the seeded admin ID so
// the caller can use it as the implicit actor for ipwhitelist/none modes.
func (s *Store) SeedAdminIfEmpty(preferredUsername, preferredPassword string) (uint, error) {
	var count int64
	if err := s.db.Model(&model.User{}).Count(&count).Error; err != nil {
		return 0, err
	}
	if count > 0 {
		return s.firstAdminID()
	}

	username := preferredUsername
	if username == "" {
		username = DefaultAdminUsername
	}
	pw := preferredPassword
	usedFallback := pw == ""
	if usedFallback {
		pw = DefaultAdminPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash admin password: %w", err)
	}
	now := time.Now()
	u := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         model.RoleAdmin,
		Disabled:     false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.db.Create(u).Error; err != nil {
		return 0, fmt.Errorf("seed admin: %w", err)
	}
	if usedFallback {
		log.Printf("[WARN] seeded default admin user %q with password %q - please change it immediately via the UI", username, DefaultAdminPassword)
	} else {
		log.Printf("seeded admin user %q from PORTPASS_ADMIN_PASSWORD", username)
	}

	if err := s.db.Model(&model.Rule{}).
		Where("user_id IS NULL OR user_id = 0 OR created_by = '' OR created_by = ?", "local").
		Updates(map[string]any{"user_id": u.ID, "created_by": u.Username}).Error; err != nil {
		return 0, fmt.Errorf("backfill legacy rules: %w", err)
	}
	return u.ID, nil
}

// firstAdminID returns the lowest-ID active admin; used as the implicit
// actor when the request is made under ipwhitelist/none auth modes.
func (s *Store) firstAdminID() (uint, error) {
	var u model.User
	err := s.db.Where("role = ? AND disabled = ?", model.RoleAdmin, false).
		Order("id ASC").First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return u.ID, nil
}

// DB returns the underlying *gorm.DB for callers that need advanced queries
// (pagination, joins, etc.) without re-implementing them on the Store.
func (s *Store) DB() *gorm.DB { return s.db }

// SeedPresetPorts inserts the default preset list when the table is empty. It
// is idempotent across restarts so operators can freely tweak the table
// without being overwritten.
func (s *Store) SeedPresetPorts() error {
	var count int64
	if err := s.db.Model(&model.PresetPort{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []model.PresetPort{
		{Name: "SSH", Port: 22, Protocol: model.ProtoTCP, Sort: 1},
		{Name: "RDP", Port: 3389, Protocol: model.ProtoTCP, Sort: 2},
		{Name: "HTTP", Port: 80, Protocol: model.ProtoTCP, Sort: 3},
		{Name: "HTTPS", Port: 443, Protocol: model.ProtoTCP, Sort: 4},
		{Name: "MySQL", Port: 3306, Protocol: model.ProtoTCP, Sort: 5},
		{Name: "PostgreSQL", Port: 5432, Protocol: model.ProtoTCP, Sort: 6},
		{Name: "Redis", Port: 6379, Protocol: model.ProtoTCP, Sort: 7},
		{Name: "MongoDB", Port: 27017, Protocol: model.ProtoTCP, Sort: 8},
	}
	return s.db.Create(&defaults).Error
}

// CreateRule inserts a new rule and populates its CommentTag once the ID is
// known. The tag is what downstream firewall drivers use to recognise their
// own rules on reconciliation.
func (s *Store) CreateRule(r *model.Rule) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		r.CommentTag = fmt.Sprintf("portpass:%d", r.ID)
		return tx.Model(r).Update("comment_tag", r.CommentTag).Error
	})
}

// UpdateRule persists the full entity; callers typically update status,
// driver_ref, expire_at or terminated_at.
func (s *Store) UpdateRule(r *model.Rule) error {
	return s.db.Save(r).Error
}

// GetRule fetches a single rule by ID.
func (s *Store) GetRule(id uint) (*model.Rule, error) {
	var r model.Rule
	if err := s.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// ListActiveRules returns rules currently in the pending or active state. The
// lifecycle manager uses this at boot to restore scheduled timers.
func (s *Store) ListActiveRules() ([]model.Rule, error) {
	var out []model.Rule
	err := s.db.Where("status IN ?", []string{model.StatusPending, model.StatusActive}).
		Order("created_at DESC").Find(&out).Error
	return out, err
}

// ListActiveByIP returns the currently-active rules created from a specific
// source IP. Used by the rate-limiter and UI filters.
func (s *Store) ListActiveByIP(ip string) ([]model.Rule, error) {
	var out []model.Rule
	err := s.db.Where("created_ip = ? AND status = ?", ip, model.StatusActive).Find(&out).Error
	return out, err
}

// ListActiveByUserIP scopes the concurrency quota to a single (user, ip)
// tuple so two different users sharing the same NAT address don't evict
// each other's rules. Falls back to IP-only when uid is zero.
func (s *Store) ListActiveByUserIP(uid uint, ip string) ([]model.Rule, error) {
	var out []model.Rule
	q := s.db.Where("created_ip = ? AND status = ?", ip, model.StatusActive)
	if uid != 0 {
		q = q.Where("user_id = ?", uid)
	}
	err := q.Find(&out).Error
	return out, err
}

// ListRulesByUser returns every rule owned by a user; used when deleting
// or auditing a user account.
func (s *Store) ListRulesByUser(uid uint) ([]model.Rule, error) {
	var out []model.Rule
	err := s.db.Where("user_id = ?", uid).Find(&out).Error
	return out, err
}

// ListAllRules returns every row with optional filters; used by the rules
// page and history page.
func (s *Store) ListAllRules(filter RuleFilter) ([]model.Rule, int64, error) {
	q := s.db.Model(&model.Rule{})
	if len(filter.Statuses) > 0 {
		q = q.Where("status IN ?", filter.Statuses)
	}
	if filter.Port != 0 {
		q = q.Where("port = ?", filter.Port)
	}
	if filter.IP != "" {
		q = q.Where("source_ip = ? OR created_ip = ?", filter.IP, filter.IP)
	}
	if filter.UserID != 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if !filter.From.IsZero() {
		q = q.Where("created_at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		q = q.Where("created_at <= ?", filter.To)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q = q.Order("created_at DESC")
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit).Offset(filter.Offset)
	}
	var out []model.Rule
	if err := q.Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// RuleFilter is the shared filter payload for listing operations.
type RuleFilter struct {
	Statuses []string
	Port     int
	IP       string
	UserID   uint
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
}

// ListPresetPorts returns all preset ports ordered by Sort.
func (s *Store) ListPresetPorts() ([]model.PresetPort, error) {
	var out []model.PresetPort
	err := s.db.Order("sort_order ASC, id ASC").Find(&out).Error
	return out, err
}

// ListUserAllowedPresets returns only presets marked UserAllowed, used to
// render the quick-button palette for non-admin users.
func (s *Store) ListUserAllowedPresets() ([]model.PresetPort, error) {
	var out []model.PresetPort
	err := s.db.Where("user_allowed = ?", true).
		Order("sort_order ASC, id ASC").Find(&out).Error
	return out, err
}

// FindPresetForUser finds a user-allowed preset that matches (port, proto).
// A preset with Protocol=both satisfies either tcp or udp requests; a
// requested proto of "both" may only match a both-preset. Returns nil when
// no matching preset exists (meaning the port is not user-allowed).
func (s *Store) FindPresetForUser(port int, proto string) (*model.PresetPort, error) {
	var ps []model.PresetPort
	if err := s.db.Where("port = ? AND user_allowed = ?", port, true).Find(&ps).Error; err != nil {
		return nil, err
	}
	for i := range ps {
		p := &ps[i]
		if p.Protocol == proto || p.Protocol == model.ProtoBoth {
			return p, nil
		}
	}
	return nil, nil
}

// UpsertPresetPort creates or updates a preset.
func (s *Store) UpsertPresetPort(p *model.PresetPort) error {
	return s.db.Save(p).Error
}

// DeletePresetPort removes a preset by ID.
func (s *Store) DeletePresetPort(id uint) error {
	return s.db.Delete(&model.PresetPort{}, id).Error
}

// ------------------------- users -------------------------

// CreateUser inserts a new user row. The caller is responsible for hashing
// the password (the model expects PasswordHash already to be a bcrypt
// digest).
func (s *Store) CreateUser(u *model.User) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	return s.db.Create(u).Error
}

// GetUserByID looks up a user by primary key; returns (nil, nil) when
// absent so callers can distinguish from real errors.
func (s *Store) GetUserByID(id uint) (*model.User, error) {
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetUserByUsername looks up a user by username. Used during login.
func (s *Store) GetUserByUsername(name string) (*model.User, error) {
	var u model.User
	if err := s.db.Where("username = ?", name).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// ListUsers returns every user row in creation order; PasswordHash stays
// on the struct but is marked json:"-" so it never leaks over the wire.
func (s *Store) ListUsers() ([]model.User, error) {
	var out []model.User
	err := s.db.Order("id ASC").Find(&out).Error
	return out, err
}

// UpdateUserFields selectively patches role / disabled; the zero-valued
// fields in the map are skipped by GORM's Updates call so callers can
// control which columns get touched.
func (s *Store) UpdateUserFields(id uint, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	return s.db.Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
}

// SetUserPasswordHash overwrites a user's bcrypt hash.
func (s *Store) SetUserPasswordHash(id uint, hash string) error {
	return s.db.Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{"password_hash": hash, "updated_at": time.Now()}).Error
}

// DeleteUser hard-deletes a user row. The API layer is responsible for
// invariants (no self-delete, keep at least one active admin, revoke their
// rules) before calling this.
func (s *Store) DeleteUser(id uint) error {
	return s.db.Delete(&model.User{}, id).Error
}

// CountActiveAdmins returns the number of enabled admin accounts. The API
// layer uses it to prevent actions that would leave the system adminless.
func (s *Store) CountActiveAdmins() (int64, error) {
	var n int64
	err := s.db.Model(&model.User{}).
		Where("role = ? AND disabled = ?", model.RoleAdmin, false).
		Count(&n).Error
	return n, err
}

// GetSetting fetches a setting value or returns fallback when missing.
func (s *Store) GetSetting(key, fallback string) (string, error) {
	var row model.Setting
	if err := s.db.First(&row, "key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fallback, nil
		}
		return "", err
	}
	return row.Value, nil
}

// SetSetting upserts a key/value pair.
func (s *Store) SetSetting(key, value string) error {
	now := time.Now()
	return s.db.Save(&model.Setting{Key: key, Value: value, UpdatedAt: now}).Error
}

// ListSettings returns every setting row (order-insensitive).
func (s *Store) ListSettings() ([]model.Setting, error) {
	var out []model.Setting
	err := s.db.Find(&out).Error
	return out, err
}

// WriteAudit appends a single audit log entry; errors are intentionally
// returned instead of logged so the caller controls severity.
func (s *Store) WriteAudit(entry *model.AuditLog) error {
	entry.CreatedAt = time.Now()
	return s.db.Create(entry).Error
}

// ListAudit returns the latest audit entries, subject to simple filters.
func (s *Store) ListAudit(filter RuleFilter) ([]model.AuditLog, int64, error) {
	q := s.db.Model(&model.AuditLog{})
	if !filter.From.IsZero() {
		q = q.Where("created_at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		q = q.Where("created_at <= ?", filter.To)
	}
	if filter.IP != "" {
		q = q.Where("actor_ip = ?", filter.IP)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q = q.Order("created_at DESC")
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit).Offset(filter.Offset)
	}
	var out []model.AuditLog
	if err := q.Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// PurgeHistory deletes audit rows older than retention days. Called by the
// lifecycle housekeeping tick.
func (s *Store) PurgeHistory(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("created_at < ?", cutoff).Delete(&model.AuditLog{}).Error
}
