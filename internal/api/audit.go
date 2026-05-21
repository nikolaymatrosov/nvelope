package api

import (
	"context"
	"log/slog"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// recordAudit appends one audit_log row for a privileged action. It reads the
// actor id + kind from the request context and falls through silently if the
// audit writer is unset (test servers that don't wire it). A recording
// failure is logged but never propagated — the primary action has already
// succeeded and the caller should still see its result.
func (s *Server) recordAudit(ctx context.Context, action, target string,
	metadata map[string]any) {

	if s.audit == nil {
		return
	}
	tenantID := tenantFromContext(ctx).ID
	if tenantID == "" {
		return
	}
	rec := iamdomain.NewAuditRecord(
		tenantID, actorIDFromContext(ctx), actorKindFromContext(ctx),
		action, target, metadata,
	)
	if err := s.audit.Record(ctx, tenantID, rec); err != nil && s.logger != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "audit record failed",
			append(requestAttrs(ctx),
				slog.String("action", action),
				slog.String("target", target),
				slog.String("error", err.Error()),
			)...,
		)
	}
}
