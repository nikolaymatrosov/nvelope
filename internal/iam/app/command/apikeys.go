package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// IssueAPIKey is the request to issue a scoped API key.
type IssueAPIKey struct {
	TenantID    string
	ActorID     string
	Name        string
	Permissions []string
}

// IssueAPIKeyResult carries the new key's id and its raw token — surfaced once,
// never recoverable afterwards.
type IssueAPIKeyResult struct {
	KeyID string
	Token string
}

// IssueAPIKeyHandler handles the IssueAPIKey command.
type IssueAPIKeyHandler struct {
	keys  domain.APIKeyRepository
	audit domain.AuditRepository
}

// NewIssueAPIKeyHandler builds the handler, failing fast on a nil dependency.
func NewIssueAPIKeyHandler(keys domain.APIKeyRepository,
	audit domain.AuditRepository) IssueAPIKeyHandler {
	if keys == nil || audit == nil {
		panic("nil dependency")
	}
	return IssueAPIKeyHandler{keys: keys, audit: audit}
}

// Handle validates the scoped permission set, mints a raw key, and persists the
// key as a hash.
func (h IssueAPIKeyHandler) Handle(ctx context.Context, cmd IssueAPIKey) (IssueAPIKeyResult, error) {
	perms, err := domain.ParsePermissions(cmd.Permissions)
	if err != nil {
		return IssueAPIKeyResult{}, err
	}
	raw, err := token.New()
	if err != nil {
		return IssueAPIKeyResult{}, err
	}
	key, err := domain.NewAPIKey(cmd.TenantID, cmd.Name, token.Hash(raw), perms, cmd.ActorID)
	if err != nil {
		return IssueAPIKeyResult{}, err
	}
	id, err := h.keys.Add(ctx, cmd.TenantID, key)
	if err != nil {
		return IssueAPIKeyResult{}, err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "apikey.issue", id,
		map[string]any{"name": cmd.Name})
	return IssueAPIKeyResult{KeyID: id, Token: raw}, nil
}

// RevokeAPIKey is the request to revoke an API key.
type RevokeAPIKey struct {
	TenantID string
	ActorID  string
	KeyID    string
}

// RevokeAPIKeyHandler handles the RevokeAPIKey command.
type RevokeAPIKeyHandler struct {
	keys  domain.APIKeyRepository
	audit domain.AuditRepository
}

// NewRevokeAPIKeyHandler builds the handler, failing fast on a nil dependency.
func NewRevokeAPIKeyHandler(keys domain.APIKeyRepository,
	audit domain.AuditRepository) RevokeAPIKeyHandler {
	if keys == nil || audit == nil {
		panic("nil dependency")
	}
	return RevokeAPIKeyHandler{keys: keys, audit: audit}
}

// Handle revokes the API key.
func (h RevokeAPIKeyHandler) Handle(ctx context.Context, cmd RevokeAPIKey) error {
	if err := h.keys.Revoke(ctx, cmd.TenantID, cmd.KeyID); err != nil {
		return err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "apikey.revoke", cmd.KeyID, nil)
	return nil
}
