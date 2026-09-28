package postgres

import (
	"context"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

const sessionColumns = `id, work_item_id, phase, opencode_session, status, agent, prompt,
	output, error, exit_code, started_at, completed_at`

func (s *Store) CreateSession(ctx context.Context, sess *domain.Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (`+sessionColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12)`,
		string(sess.ID), string(sess.WorkItemID), string(sess.Phase), sess.OpenCodeSession,
		string(sess.Status), sess.Agent, sess.Prompt, outputJSON(sess.Output), sess.Error,
		sess.ExitCode, sess.StartedAt, sess.CompletedAt)
	return err
}

func (s *Store) GetSession(ctx context.Context, id domain.SessionID) (*domain.Session, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, string(id))
	sess, err := scanSession(row)
	if err != nil {
		return nil, mapReadErr(err)
	}
	return sess, nil
}

func (s *Store) UpdateSession(ctx context.Context, sess *domain.Session) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions SET
			opencode_session=$2, status=$3, agent=$4, prompt=$5, output=$6::jsonb,
			error=$7, exit_code=$8, completed_at=$9
		WHERE id=$1`,
		string(sess.ID), sess.OpenCodeSession, string(sess.Status), sess.Agent, sess.Prompt,
		outputJSON(sess.Output), sess.Error, sess.ExitCode, sess.CompletedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListSessions(ctx context.Context, workItemID domain.WorkItemID) ([]*domain.Session, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE work_item_id=$1 ORDER BY started_at ASC`, string(workItemID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func scanSession(row rowScanner) (*domain.Session, error) {
	var (
		sess                             domain.Session
		id, workItemID, phase, ocSession string
		status, agent, prompt, errMsg    string
		outRaw                           []byte
	)
	if err := row.Scan(&id, &workItemID, &phase, &ocSession, &status, &agent, &prompt,
		&outRaw, &errMsg, &sess.ExitCode, &sess.StartedAt, &sess.CompletedAt); err != nil {
		return nil, err
	}
	sess.ID = domain.SessionID(id)
	sess.WorkItemID = domain.WorkItemID(workItemID)
	sess.Phase = domain.Phase(phase)
	sess.OpenCodeSession = ocSession
	sess.Status = domain.SessionStatus(status)
	sess.Agent = agent
	sess.Prompt = prompt
	sess.Error = errMsg
	if len(outRaw) > 0 {
		out := &domain.SessionOutput{}
		if err := unmarshal(outRaw, out); err != nil {
			return nil, err
		}
		sess.Output = out
	}
	return &sess, nil
}

func outputJSON(o *domain.SessionOutput) string {
	if o == nil {
		return "null"
	}
	return mustJSON(o)
}
