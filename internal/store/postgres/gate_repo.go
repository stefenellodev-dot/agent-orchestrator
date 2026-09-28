package postgres

import (
	"context"
	"time"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

const gateColumns = `id, work_item_id, phase, required_approvers, status, payload, created_at, resolved_at`

func (s *Store) CreateGate(ctx context.Context, g *domain.Gate) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO gates (`+gateColumns+`)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6::jsonb,$7,$8)`,
		string(g.ID), string(g.WorkItemID), string(g.Phase), mustJSON(g.RequiredApprovers),
		string(g.Status), mustJSON(g.Payload), g.CreatedAt, g.ResolvedAt)
	return mapWriteErr(err)
}

func (s *Store) GetGate(ctx context.Context, id domain.GateID) (*domain.Gate, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+gateColumns+` FROM gates WHERE id=$1`, string(id))
	g, err := scanGate(row)
	if err != nil {
		return nil, mapReadErr(err)
	}
	return s.attachApprovals(ctx, g)
}

func (s *Store) GetGateByWorkItem(ctx context.Context, workItemID domain.WorkItemID) (*domain.Gate, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+gateColumns+` FROM gates WHERE work_item_id=$1`, string(workItemID))
	g, err := scanGate(row)
	if err != nil {
		return nil, mapReadErr(err)
	}
	return s.attachApprovals(ctx, g)
}

func (s *Store) UpdateGate(ctx context.Context, g *domain.Gate) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE gates SET
			required_approvers=$2::jsonb, status=$3, payload=$4::jsonb, resolved_at=$5
		WHERE id=$1`,
		string(g.ID), mustJSON(g.RequiredApprovers), string(g.Status), mustJSON(g.Payload), g.ResolvedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM approvals WHERE gate_id=$1`, string(g.ID)); err != nil {
		return err
	}
	for _, a := range g.Approvals {
		var auth any
		if a.Authorization != nil {
			auth = mustJSON(a.Authorization)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO approvals (gate_id, username, decision, comment, authorization_record, at)
			VALUES ($1,$2,$3,$4,$5::jsonb,$6)`,
			string(g.ID), a.User, string(a.Decision), a.Comment, auth, a.At); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) attachApprovals(ctx context.Context, g *domain.Gate) (*domain.Gate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT username, decision, comment, authorization_record, at
		FROM approvals WHERE gate_id=$1 ORDER BY at ASC`, string(g.ID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	g.Approvals = nil
	for rows.Next() {
		var (
			a        domain.Approval
			user     string
			decision string
			comment  string
			authRaw  []byte
		)
		if err := rows.Scan(&user, &decision, &comment, &authRaw, &a.At); err != nil {
			return nil, err
		}
		a.User = user
		a.Decision = domain.ApprovalDecision(decision)
		a.Comment = comment
		if len(authRaw) > 0 {
			auth := &domain.AuthorizationRecord{}
			if err := unmarshal(authRaw, auth); err != nil {
				return nil, err
			}
			a.Authorization = auth
		}
		g.Approvals = append(g.Approvals, a)
	}
	return g, rows.Err()
}

func scanGate(row rowScanner) (*domain.Gate, error) {
	var (
		g              domain.Gate
		id, workItemID string
		phase, status  string
		approversRaw   []byte
		payloadRaw     []byte
		resolved       *time.Time
	)
	if err := row.Scan(&id, &workItemID, &phase, &approversRaw, &status, &payloadRaw, &g.CreatedAt, &resolved); err != nil {
		return nil, err
	}
	g.ID = domain.GateID(id)
	g.WorkItemID = domain.WorkItemID(workItemID)
	g.Phase = domain.Phase(phase)
	g.Status = domain.GateStatus(status)
	g.ResolvedAt = resolved
	if len(approversRaw) > 0 {
		_ = unmarshal(approversRaw, &g.RequiredApprovers)
	}
	if len(payloadRaw) > 0 {
		_ = unmarshal(payloadRaw, &g.Payload)
	}
	return &g, nil
}
