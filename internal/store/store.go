package store

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/portset"
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
	// Silence the noisy "record not found" warnings - they're expected
	// for many code paths (e.g. lookup-or-create, optional KV reads).
	gormLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormLogger,
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
		&model.PresetCategory{},
		&model.PresetPort{},
		&model.ProtectedPort{},
		&model.UserAllowedRange{},
		&model.Setting{},
		&model.AuditLog{},
		&model.User{},
		&model.LoginAttempt{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	s := &Store{db: db}
	if err := s.backfillPortGroups(); err != nil {
		return nil, fmt.Errorf("backfill ports: %w", err)
	}
	return s, nil
}

// backfillPortGroups is a one-shot migration that copies the legacy
// single-port column into the new `ports` canonical string column.
// Rows that already have a non-empty `ports` value are left alone so
// the migration is idempotent across restarts.
func (s *Store) backfillPortGroups() error {
	var rules []model.Rule
	if err := s.db.Where("ports = '' OR ports IS NULL").Find(&rules).Error; err != nil {
		return err
	}
	for i := range rules {
		r := &rules[i]
		if r.Port > 0 {
			r.Ports = strconv.Itoa(r.Port)
			if err := s.db.Model(r).Update("ports", r.Ports).Error; err != nil {
				return err
			}
		}
	}
	var presets []model.PresetPort
	if err := s.db.Where("ports = '' OR ports IS NULL").Find(&presets).Error; err != nil {
		return err
	}
	for i := range presets {
		p := &presets[i]
		if p.Port > 0 {
			p.Ports = strconv.Itoa(p.Port)
			if err := s.db.Model(p).Update("ports", p.Ports).Error; err != nil {
				return err
			}
		}
	}
	return nil
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

// SeedPresetCategories inserts the six built-in categories the first time
// the table is empty. The keys/icons mirror the legacy frontend
// categorize() heuristic so existing presets keep their auto-detected
// grouping after the migration. Subsequent boots are no-ops; admins are
// free to rename, re-icon, or hide categories without being overwritten.
func (s *Store) SeedPresetCategories() error {
	var count int64
	if err := s.db.Model(&model.PresetCategory{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []model.PresetCategory{
		{Key: "remote", Icon: "🔐", Sort: 1, Builtin: true},
		{Key: "web", Icon: "🌐", Sort: 2, Builtin: true},
		{Key: "db", Icon: "🗄️", Sort: 3, Builtin: true},
		{Key: "mq", Icon: "📬", Sort: 4, Builtin: true},
		{Key: "game", Icon: "🎮", Sort: 5, Builtin: true},
		{Key: "misc", Icon: "🔌", Sort: 6, Builtin: true},
	}
	return s.db.Create(&defaults).Error
}

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
		{Name: "SSH", Port: 22, Ports: "22", Protocol: model.ProtoTCP, Sort: 1},
		{Name: "RDP", Port: 3389, Ports: "3389", Protocol: model.ProtoTCP, Sort: 2},
		{Name: "HTTP", Port: 80, Ports: "80", Protocol: model.ProtoTCP, Sort: 3},
		{Name: "HTTPS", Port: 443, Ports: "443", Protocol: model.ProtoTCP, Sort: 4},
		{Name: "MySQL", Port: 3306, Ports: "3306", Protocol: model.ProtoTCP, Sort: 5},
		{Name: "PostgreSQL", Port: 5432, Ports: "5432", Protocol: model.ProtoTCP, Sort: 6},
		{Name: "Redis", Port: 6379, Ports: "6379", Protocol: model.ProtoTCP, Sort: 7},
		{Name: "MongoDB", Port: 27017, Ports: "27017", Protocol: model.ProtoTCP, Sort: 8},
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

// NotifyChannel is a string-typed enum used by the store APIs to
// identify which per-channel sent_at column to query / update. The
// rule row tracks browser and ntfy independently because the operator
// might pick the "browser + ntfy" channel mode, in which case both
// pipelines must fire even when one of them happens to mark the
// row first.
type NotifyChannel string

const (
	NotifyChannelBrowser NotifyChannel = "browser"
	NotifyChannelNtfy    NotifyChannel = "ntfy"
)

func (c NotifyChannel) sentColumn() string {
	if c == NotifyChannelNtfy {
		return "notify_sent_ntfy_at"
	}
	return "notify_sent_browser_at"
}

// ListPendingNotify returns rules eligible for an "imminent expiry"
// notification on the given channel: status active|pending, opt-in via
// NotifyEnabled, not yet notified on this channel (the per-channel
// sent_at IS NULL), and the configured lead time has elapsed (now >=
// expire_at - lead). The window upper bound (expire_at > now) keeps
// already-expired rules out so we never push a useless "rule expired"
// message after the fact. uid==0 means "no user filter" (used by the
// ntfy watcher); a non-zero uid restricts the result to that user
// (used by the browser-poll endpoint).
func (s *Store) ListPendingNotify(uid uint, channel NotifyChannel, now time.Time) ([]model.Rule, error) {
	var out []model.Rule
	col := channel.sentColumn()
	q := s.db.Where("status IN ? AND notify_enabled = ?",
		[]string{model.StatusActive, model.StatusPending}, true).
		Where(col + " IS NULL").
		Where("expire_at > ?", now).
		// SQLite stores time.Time as ISO-8601 strings; arithmetic via
		// julianday lets us compare "expire_at - lead seconds" against
		// `now` without depending on the SQLite extension.
		Where("(julianday(expire_at) - (notify_lead_seconds * 1.0 / 86400.0)) <= julianday(?)", now)
	if uid != 0 {
		q = q.Where("user_id = ?", uid)
	}
	err := q.Order("expire_at ASC").Find(&out).Error
	return out, err
}

// MarkNotifySent stamps the channel-scoped sent_at column to `at` on
// every rule in ids that is owned by uid (a guard so an attacker
// forging another user's rule id cannot suppress that user's
// notifications). uid==0 disables the owner check so the ntfy watcher
// can mark any rule. Returns the number of rows actually updated.
func (s *Store) MarkNotifySent(ids []uint, channel NotifyChannel, uid uint, at time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	col := channel.sentColumn()
	q := s.db.Model(&model.Rule{}).
		Where("id IN ? AND "+col+" IS NULL", ids)
	if uid != 0 {
		q = q.Where("user_id = ?", uid)
	}
	res := q.Update(col, at)
	return res.RowsAffected, res.Error
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

// FindPresetsForPortSet returns every user-allowed preset whose port
// group is a superset of the requested set under a compatible protocol.
// The policy layer then picks the one with the lowest MaxDurationSec
// (most restrictive) to bound the request. Returning a slice lets the
// caller pick whichever tie-breaker suits its purpose; an empty slice
// means the request is not user-allowed.
func (s *Store) FindPresetsForPortSet(want portset.Set, proto string) ([]model.PresetPort, error) {
	all, err := s.ListUserAllowedPresets()
	if err != nil {
		return nil, err
	}
	var matches []model.PresetPort
	for _, p := range all {
		if !protoCompatible(p.Protocol, proto) {
			continue
		}
		ps, err := portset.Parse(p.Ports)
		if err != nil || ps.Empty() {
			continue
		}
		if ps.ContainsSet(want) {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

// protoCompatible reports whether a rule with requested protocol `req`
// is satisfied by a slot whose protocol is `slot`. A slot="both" always
// matches; a request "both" must have a matching "both" slot; same
// protocol matches itself.
func protoCompatible(slot, req string) bool {
	if slot == model.ProtoBoth {
		return true
	}
	return slot == req
}

// UpsertPresetPort creates or updates a preset.
func (s *Store) UpsertPresetPort(p *model.PresetPort) error {
	return s.db.Save(p).Error
}

// DeletePresetPort removes a preset by ID.
func (s *Store) DeletePresetPort(id uint) error {
	return s.db.Delete(&model.PresetPort{}, id).Error
}

// ListPresetCategories returns all categories ordered by Sort then ID so
// the UI can render a stable list.
func (s *Store) ListPresetCategories() ([]model.PresetCategory, error) {
	var out []model.PresetCategory
	err := s.db.Order("sort_order ASC, id ASC").Find(&out).Error
	return out, err
}

// GetPresetCategory loads one category by ID; returns (nil, nil) when
// absent so callers can distinguish from a real error.
func (s *Store) GetPresetCategory(id uint) (*model.PresetCategory, error) {
	var c model.PresetCategory
	if err := s.db.First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// UpsertPresetCategory creates or updates a category. Builtin protection
// (no creating new builtins, no flipping existing builtins to false) is
// enforced by the API layer before this call.
func (s *Store) UpsertPresetCategory(c *model.PresetCategory) error {
	return s.db.Save(c).Error
}

// DeletePresetCategory removes a category and detaches every preset
// pointing at it (CategoryID becomes NULL, so the preset falls back to
// the heuristic auto-detection on next render). Wrapped in a single
// transaction so a partial failure leaves no orphaned references.
func (s *Store) DeletePresetCategory(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.PresetPort{}).
			Where("category_id = ?", id).
			Update("category_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&model.PresetCategory{}, id).Error
	})
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
	v, ok, err := s.LookupSetting(key)
	if err != nil {
		return "", err
	}
	if !ok {
		return fallback, nil
	}
	return v, nil
}

// LookupSetting reports whether a key exists in the settings KV table
// and returns its value when so. Use this from runtime.LoadFromKV to
// distinguish "missing" (use boot default) from "empty string"
// (operator deliberately blanked the field, e.g. ntfy_token).
//
// Implementation note: we use Limit(1).Find() instead of First() so the
// gorm logger never produces a noisy ErrRecordNotFound entry for the
// expected miss case during boot.
func (s *Store) LookupSetting(key string) (string, bool, error) {
	var rows []model.Setting
	if err := s.db.Where("key = ?", key).Limit(1).Find(&rows).Error; err != nil {
		return "", false, err
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	return rows[0].Value, true, nil
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

// ------------------------- protected ports -------------------------

// ListProtectedPorts returns every protected-port row.
func (s *Store) ListProtectedPorts() ([]model.ProtectedPort, error) {
	var out []model.ProtectedPort
	err := s.db.Order("id ASC").Find(&out).Error
	return out, err
}

// FindProtectedOverlap returns the first protected-port row whose port
// group intersects with `want` under a compatible protocol. Returns
// (nil, nil) when nothing overlaps.
func (s *Store) FindProtectedOverlap(want portset.Set, proto string) (*model.ProtectedPort, error) {
	rows, err := s.ListProtectedPorts()
	if err != nil {
		return nil, err
	}
	for i := range rows {
		p := &rows[i]
		if !protoCompatible(p.Protocol, proto) && !protoCompatible(proto, p.Protocol) {
			continue
		}
		ps, err := portset.Parse(p.Ports)
		if err != nil || ps.Empty() {
			continue
		}
		if ps.Overlaps(want) {
			return p, nil
		}
	}
	return nil, nil
}

// UpsertProtectedPort creates or updates a protected-port row.
func (s *Store) UpsertProtectedPort(p *model.ProtectedPort) error {
	return s.db.Save(p).Error
}

// DeleteProtectedPort removes a protected-port row.
func (s *Store) DeleteProtectedPort(id uint) error {
	return s.db.Delete(&model.ProtectedPort{}, id).Error
}

// ------------------------- user allowed ranges -------------------------

// ListUserAllowedRanges returns the per-user override list.
func (s *Store) ListUserAllowedRanges(userID uint) ([]model.UserAllowedRange, error) {
	var out []model.UserAllowedRange
	err := s.db.Where("user_id = ?", userID).Order("id ASC").Find(&out).Error
	return out, err
}

// HasPersonalRanges reports whether a user has at least one allowed-range
// row; the policy layer uses this to switch from preset.user_allowed
// fallback to per-user override mode.
func (s *Store) HasPersonalRanges(userID uint) (bool, error) {
	var n int64
	err := s.db.Model(&model.UserAllowedRange{}).Where("user_id = ?", userID).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountPersonalRanges is the Store counterpart exposed to the API for
// display-only purposes (e.g. the users table's policy column).
func (s *Store) CountPersonalRanges(userID uint) (int64, error) {
	var n int64
	err := s.db.Model(&model.UserAllowedRange{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

// FindUserAllowedForRequest returns the user's matching range (superset
// of `want` under a compatible protocol). Returns (nil, nil) when
// nothing covers the request.
func (s *Store) FindUserAllowedForRequest(userID uint, want portset.Set, proto string) (*model.UserAllowedRange, error) {
	rows, err := s.ListUserAllowedRanges(userID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		r := &rows[i]
		if !protoCompatible(r.Protocol, proto) {
			continue
		}
		ps, err := portset.Parse(r.Ports)
		if err != nil || ps.Empty() {
			continue
		}
		if ps.ContainsSet(want) {
			return r, nil
		}
	}
	return nil, nil
}

// UpsertUserAllowedRange creates or updates a personal range row.
func (s *Store) UpsertUserAllowedRange(r *model.UserAllowedRange) error {
	return s.db.Save(r).Error
}

// DeleteUserAllowedRange removes a personal range row.
func (s *Store) DeleteUserAllowedRange(id uint) error {
	return s.db.Delete(&model.UserAllowedRange{}, id).Error
}

// ClearUserAllowedRanges removes every personal range row for a user.
// The policy layer then reverts to preset.user_allowed fallback.
func (s *Store) ClearUserAllowedRanges(userID uint) error {
	return s.db.Where("user_id = ?", userID).Delete(&model.UserAllowedRange{}).Error
}

// ------------------------- login attempts -------------------------

// RecordLoginAttempt persists one login attempt (success or failure).
// Errors are returned so the caller can log without blocking auth flow,
// but callers typically ignore them (the audit log is best-effort).
func (s *Store) RecordLoginAttempt(a *model.LoginAttempt) error {
	a.CreatedAt = time.Now()
	return s.db.Create(a).Error
}

// CountLoginFailuresByIP returns the number of failed attempts from ip
// since `since`. Used by the brute-force limiter.
func (s *Store) CountLoginFailuresByIP(ip string, since time.Time) (int64, error) {
	var n int64
	err := s.db.Model(&model.LoginAttempt{}).
		Where("client_ip = ? AND success = ? AND created_at >= ?", ip, false, since).
		Count(&n).Error
	return n, err
}

// CountLoginFailuresByUsername returns the number of failed attempts for
// username since `since`. Used by the brute-force limiter.
func (s *Store) CountLoginFailuresByUsername(username string, since time.Time) (int64, error) {
	var n int64
	err := s.db.Model(&model.LoginAttempt{}).
		Where("username = ? AND success = ? AND created_at >= ?", username, false, since).
		Count(&n).Error
	return n, err
}

// CountLoginFailuresByIPSubnet returns the number of failed attempts whose
// recorded ClientIP falls inside the given CIDR prefix. Implemented via a
// SQL LIKE on the parent /N - which is cheap enough for the small per-host
// log we keep, and avoids loading every row into memory just to apply a
// netmask. Used only when LoginFailSubnetBits > 0.
func (s *Store) CountLoginFailuresByIPSubnet(prefix string, since time.Time) (int64, error) {
	parts := strings.SplitN(prefix, "/", 2)
	if len(parts) != 2 {
		return 0, nil
	}
	bits, err := strconv.Atoi(parts[1])
	if err != nil || bits <= 0 {
		return 0, nil
	}
	ip := net.ParseIP(parts[0])
	if ip == nil {
		return 0, nil
	}
	is4 := ip.To4() != nil
	matches, scanErr := s.scanIPsInRange(since, ip, bits, is4)
	if scanErr != nil {
		return 0, scanErr
	}
	if len(matches) == 0 {
		return 0, nil
	}
	var n int64
	err = s.db.Model(&model.LoginAttempt{}).
		Where("success = ? AND created_at >= ? AND client_ip IN ?", false, since, matches).
		Count(&n).Error
	return n, err
}

// scanIPsInRange returns the distinct ClientIPs already seen since
// `since` whose addresses fall within `ip/bits`. The scan is bounded
// by the (already-rare) login_attempts table size; this hot path runs
// only when subnet aggregation is explicitly enabled.
func (s *Store) scanIPsInRange(since time.Time, prefixIP net.IP, bits int, is4 bool) ([]string, error) {
	var ips []string
	err := s.db.Model(&model.LoginAttempt{}).
		Where("success = ? AND created_at >= ?", false, since).
		Distinct("client_ip").
		Pluck("client_ip", &ips).Error
	if err != nil {
		return nil, err
	}
	mask := net.CIDRMask(bits, 32)
	if !is4 {
		mask = net.CIDRMask(bits, 128)
	}
	expected := prefixIP.Mask(mask)
	out := make([]string, 0, len(ips))
	for _, raw := range ips {
		candidate := net.ParseIP(raw)
		if candidate == nil {
			continue
		}
		if is4 {
			c4 := candidate.To4()
			if c4 == nil {
				continue
			}
			if c4.Mask(mask).Equal(expected) {
				out = append(out, raw)
			}
		} else {
			if candidate.Mask(mask).Equal(expected) {
				out = append(out, raw)
			}
		}
	}
	return out, nil
}

// ListLoginAttempts returns recent login attempts. When username is empty
// all rows are returned (admin view); otherwise the query is scoped to
// that user (self-service view).
func (s *Store) ListLoginAttempts(username string, limit int) ([]model.LoginAttempt, error) {
	q := s.db.Model(&model.LoginAttempt{})
	if username != "" {
		q = q.Where("username = ?", username)
	}
	if limit <= 0 {
		limit = 100
	}
	var out []model.LoginAttempt
	err := q.Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

// LastSuccessfulLogin returns the most recent successful login for username.
// Callers invoke this BEFORE recording the current success so the returned
// row is the previous session. Returns (nil, nil) when the user has never
// logged in successfully.
func (s *Store) LastSuccessfulLogin(username string) (*model.LoginAttempt, error) {
	var row model.LoginAttempt
	err := s.db.Where("username = ? AND success = ?", username, true).
		Order("created_at DESC").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// PurgeLoginAttempts trims the login_attempts table. Failed rows live
// `failRetentionDays` days (default 30), successful rows live indefinitely
// (useful for auditing) unless `successRetentionDays` > 0 is given.
func (s *Store) PurgeLoginAttempts(failRetentionDays, successRetentionDays int) error {
	if failRetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -failRetentionDays)
		if err := s.db.Where("success = ? AND created_at < ?", false, cutoff).
			Delete(&model.LoginAttempt{}).Error; err != nil {
			return err
		}
	}
	if successRetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -successRetentionDays)
		if err := s.db.Where("success = ? AND created_at < ?", true, cutoff).
			Delete(&model.LoginAttempt{}).Error; err != nil {
			return err
		}
	}
	return nil
}
