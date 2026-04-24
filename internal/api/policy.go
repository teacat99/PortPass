package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/teacat99/PortPass/internal/auth"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/portset"
)

// ------------------------- protected ports (admin-only) -------------------------

type protectedReq struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Ports    string `json:"ports"`
	Protocol string `json:"protocol"`
	Note     string `json:"note"`
}

// handleListProtected returns the full protected-port table. Both
// admins and ordinary users can read it so the UI can render a
// "locked" badge on the home page if the user tries to type a
// matching port.
func (s *Server) handleListProtected(c *gin.Context) {
	rows, err := s.store.ListProtectedPorts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// handleUpsertProtected creates or updates a protected-port row. The
// reverse intersection check ensures a newly-added protected port does
// not silently conflict with an existing preset — the admin is shown a
// 409 and must reconcile the preset first.
func (s *Server) handleUpsertProtected(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	var req protectedReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ps, err := portset.Parse(req.Ports)
	if err != nil || ps.Empty() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ports"})
		return
	}
	proto, err := normaliseProtocol(req.Protocol)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Reverse check: does this conflict with an existing user-allowed preset?
	presets, err := s.store.ListPresetPorts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, p := range presets {
		if p.ID == req.ID {
			continue
		}
		if !p.UserAllowed {
			continue
		}
		presetSet, err := portset.Parse(p.Ports)
		if err != nil || presetSet.Empty() {
			continue
		}
		if (p.Protocol == proto || p.Protocol == model.ProtoBoth || proto == model.ProtoBoth) &&
			presetSet.Overlaps(ps) {
			c.JSON(http.StatusConflict, gin.H{
				"error": fmt.Sprintf("conflicts with user-allowed preset %q (%s)", p.Name, p.Ports),
			})
			return
		}
	}
	row := model.ProtectedPort{
		ID:       req.ID,
		Name:     req.Name,
		Ports:    ps.String(),
		Protocol: proto,
		Note:     req.Note,
	}
	if err := s.store.UpsertProtectedPort(&row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "upsert_protected", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("%s %s/%s", row.Name, row.Ports, row.Protocol),
	})
	c.JSON(http.StatusOK, row)
}

func (s *Server) handleDeleteProtected(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.DeleteProtectedPort(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "delete_protected", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("id=%d", id),
	})
	c.Status(http.StatusNoContent)
}

// ------------------------- user allowed ranges (admin-only) -------------------------

type userRangeReq struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Ports          string `json:"ports"`
	Protocol       string `json:"protocol"`
	MaxDurationSec int    `json:"max_duration_sec"`
	Note           string `json:"note"`
}

func (s *Server) handleListUserRanges(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	uid, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows, err := s.store.ListUserAllowedRanges(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) handleUpsertUserRange(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	uid, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if target, err := s.store.GetUserByID(uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	var req userRangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ps, err := portset.Parse(req.Ports)
	if err != nil || ps.Empty() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ports"})
		return
	}
	proto, err := normaliseProtocol(req.Protocol)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MaxDurationSec < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_duration_sec must be >= 0"})
		return
	}
	// Personal ranges must not cover protected ports either — the
	// policy chain would block their use anyway, but rejecting at
	// write-time is friendlier UX for the admin.
	if clash, err := s.store.FindProtectedOverlap(ps, proto); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if clash != nil {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("range overlaps protected port %q (%s)", clash.Name, clash.Ports)})
		return
	}
	row := model.UserAllowedRange{
		ID:             req.ID,
		UserID:         uid,
		Name:           req.Name,
		Ports:          ps.String(),
		Protocol:       proto,
		MaxDurationSec: req.MaxDurationSec,
		Note:           req.Note,
	}
	if err := s.store.UpsertUserAllowedRange(&row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "upsert_user_range", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("uid=%d ports=%s proto=%s", uid, row.Ports, row.Protocol),
	})
	c.JSON(http.StatusOK, row)
}

func (s *Server) handleDeleteUserRange(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	rid, err := parseID(c.Param("rid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.DeleteUserAllowedRange(rid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "delete_user_range", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("rid=%d", rid),
	})
	c.Status(http.StatusNoContent)
}

func (s *Server) handleClearUserRanges(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	uid, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.ClearUserAllowedRanges(uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, actor, _ := auth.Principal(c)
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "clear_user_ranges", Actor: actor, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("uid=%d", uid),
	})
	c.Status(http.StatusNoContent)
}
