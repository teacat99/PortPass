package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/teacat99/PortPass/internal/auth"
	"github.com/teacat99/PortPass/internal/model"
)

// minPasswordLen is the bare-minimum length we accept for user-provided
// passwords. Intentionally low so operators on small home setups are not
// frustrated; callers should pair it with the seeded-admin log warning.
const minPasswordLen = 6

// ensureAdmin aborts the request with 403 when the current principal is
// not an admin. Returns true on success so the caller can continue.
func (s *Server) ensureAdmin(c *gin.Context) bool {
	_, _, role := auth.Principal(c)
	if role != model.RoleAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		return false
	}
	return true
}

// currentUserID pulls the authenticated user id from the context. Used
// when scoping list/create operations to the caller.
func currentUserID(c *gin.Context) uint {
	id, _, _ := auth.Principal(c)
	return id
}

// ------------------------- /api/auth/me /password -------------------------

func (s *Server) handleMe(c *gin.Context) {
	id, name, role := auth.Principal(c)
	c.JSON(http.StatusOK, gin.H{
		"id":       id,
		"username": name,
		"role":     role,
		"auth_mode": string(s.cfg.AuthMode),
	})
}

type changePwdReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required"`
}

// handleChangeOwnPassword lets the signed-in user rotate their own
// password. In password auth mode we require the old password; other
// modes don't have real credentials so we skip that check.
func (s *Server) handleChangeOwnPassword(c *gin.Context) {
	var req changePwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("password must be at least %d chars", minPasswordLen)})
		return
	}
	uid := currentUserID(c)
	u, err := s.store.GetUserByID(uid)
	if err != nil || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if u.PasswordHash != "" {
		// In password mode the old password must match. In ipwhitelist /
		// none modes the system admin account may have an empty hash (no
		// login path); we still allow an admin to set an initial one.
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "old password incorrect"})
			return
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.SetUserPasswordHash(u.ID, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "change_password", Actor: u.Username, ActorIP: s.clientIP(c),
		Detail: "self",
	})
	c.Status(http.StatusOK)
}

// ------------------------- /api/users (admin-only) -------------------------

type createUserReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

func (s *Server) handleListUsers(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	us, err := s.store.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": us})
}

func (s *Server) handleCreateUser(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	if len(req.Password) < minPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("password must be at least %d chars", minPasswordLen)})
		return
	}
	role := normaliseRole(req.Role)
	if role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or user"})
		return
	}
	if existing, err := s.store.GetUserByUsername(username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
	}
	if err := s.store.CreateUser(u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "create_user", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("%s role=%s", username, role),
	})
	c.JSON(http.StatusOK, u)
}

type updateUserReq struct {
	Role     *string `json:"role,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
}

// handleUpdateUser patches role / disabled with the "at-least-one active
// admin" invariant and the "cannot modify self" rule enforced atomically
// before we touch the row.
func (s *Server) handleUpdateUser(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selfID := currentUserID(c)
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	target, err := s.store.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if id == selfID && (req.Role != nil || req.Disabled != nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot modify role/disabled on self"})
		return
	}
	fields := map[string]any{}
	newRole := target.Role
	newDisabled := target.Disabled
	if req.Role != nil {
		role := normaliseRole(*req.Role)
		if role == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or user"})
			return
		}
		newRole = role
		fields["role"] = role
	}
	if req.Disabled != nil {
		newDisabled = *req.Disabled
		fields["disabled"] = newDisabled
	}
	if len(fields) == 0 {
		c.JSON(http.StatusOK, target)
		return
	}
	// If this change reduces the set of active admins, block it when it
	// would leave the system without one.
	wasActiveAdmin := target.Role == model.RoleAdmin && !target.Disabled
	becomesInactiveAdmin := newRole != model.RoleAdmin || newDisabled
	if wasActiveAdmin && becomesInactiveAdmin {
		n, err := s.store.CountActiveAdmins()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if n <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "must keep at least one active admin"})
			return
		}
	}
	if err := s.store.UpdateUserFields(id, fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, _ := s.store.GetUserByID(id)
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "update_user", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("id=%d role=%s disabled=%v", id, newRole, newDisabled),
	})
	c.JSON(http.StatusOK, updated)
}

type resetPwdReq struct {
	NewPassword string `json:"new_password" binding:"required"`
}

func (s *Server) handleResetUserPassword(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req resetPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("password must be at least %d chars", minPasswordLen)})
		return
	}
	target, err := s.store.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.SetUserPasswordHash(id, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "reset_password", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("target=%s", target.Username),
	})
	c.Status(http.StatusOK)
}

// handleDeleteUser enforces the two invariants the spec calls out:
// (a) an admin cannot delete themselves; (b) the last active admin cannot
// be removed. Rules owned by the deleted user are revoked via the
// lifecycle manager so no firewall entries are left behind.
func (s *Server) handleDeleteUser(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selfID := currentUserID(c)
	if id == selfID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete self"})
		return
	}
	target, err := s.store.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if target.Role == model.RoleAdmin && !target.Disabled {
		n, err := s.store.CountActiveAdmins()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if n <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "must keep at least one active admin"})
			return
		}
	}

	rules, err := s.store.ListRulesByUser(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range rules {
		r := &rules[i]
		if r.Status == model.StatusActive || r.Status == model.StatusPending {
			if err := s.lifecycle.Revoke(r); err != nil {
				// Log via audit but don't abort the delete; orphaned
				// entries will be picked up by reconcile shortly.
				_ = s.store.WriteAudit(&model.AuditLog{
					Action: "delete_user_rule_revoke_failed", RuleID: r.ID,
					ActorIP: s.clientIP(c), Detail: err.Error(),
				})
			}
		}
	}
	if err := s.store.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "delete_user", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("%s (id=%d)", target.Username, id),
	})
	c.Status(http.StatusNoContent)
}

// normaliseRole maps user input to the canonical role enum; returns an
// empty string when the value is unrecognised so the caller can 400.
func normaliseRole(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "", model.RoleUser:
		return model.RoleUser
	case model.RoleAdmin:
		return model.RoleAdmin
	}
	return ""
}
