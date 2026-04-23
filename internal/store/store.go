package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/teacat99/PortPass/internal/model"
)

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
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return &Store{db: db}, nil
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

// UpsertPresetPort creates or updates a preset.
func (s *Store) UpsertPresetPort(p *model.PresetPort) error {
	return s.db.Save(p).Error
}

// DeletePresetPort removes a preset by ID.
func (s *Store) DeletePresetPort(id uint) error {
	return s.db.Delete(&model.PresetPort{}, id).Error
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
