package postgres

import (
	"context"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

const workItemColumns = `id, project, title, description, priority, status, current_phase,
	worktree_path, base_branch, base_commit_sha, assignee, metadata, created_at, updated_at`

func (s *Store) CreateWorkItem(ctx context.Context, wi *domain.WorkItem) error {
	meta := wi.Metadata
	if meta == nil {
		meta = domain.Metadata{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO work_items (`+workItemColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14)`,
		string(wi.ID), wi.Project, wi.Title, wi.Description, string(wi.Priority),
		string(wi.Status), string(wi.CurrentPhase), wi.WorktreePath, wi.BaseBranch,
		wi.BaseCommitSHA, wi.Assignee, mustJSON(meta), wi.CreatedAt, wi.UpdatedAt)
	return mapWriteErr(err)
}

func (s *Store) GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+workItemColumns+` FROM work_items WHERE id = $1`, string(id))
	wi, err := scanWorkItem(row)
	if err != nil {
		return nil, mapReadErr(err)
	}
	return wi, nil
}

func (s *Store) UpdateWorkItem(ctx context.Context, wi *domain.WorkItem) error {
	meta := wi.Metadata
	if meta == nil {
		meta = domain.Metadata{}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE work_items SET
			project=$2, title=$3, description=$4, priority=$5, status=$6, current_phase=$7,
			worktree_path=$8, base_branch=$9, base_commit_sha=$10, assignee=$11, metadata=$12::jsonb, updated_at=$13
		WHERE id=$1`,
		string(wi.ID), wi.Project, wi.Title, wi.Description, string(wi.Priority),
		string(wi.Status), string(wi.CurrentPhase), wi.WorktreePath, wi.BaseBranch,
		wi.BaseCommitSHA, wi.Assignee, mustJSON(meta), wi.UpdatedAt)
	if err != nil {
		return mapWriteErr(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListWorkItems(ctx context.Context, project string) ([]*domain.WorkItem, error) {
	query := `SELECT ` + workItemColumns + ` FROM work_items`
	args := []any{}
	if project != "" {
		query += ` WHERE project = $1`
		args = append(args, project)
	}
	query += ` ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.WorkItem
	for rows.Next() {
		wi, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wi)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorkItem(row rowScanner) (*domain.WorkItem, error) {
	var (
		wi                                             domain.WorkItem
		id, project, title, description                string
		priority, status, phase                        string
		worktreePath, baseBranch, baseCommit, assignee string
		metaRaw                                        []byte
	)
	if err := row.Scan(&id, &project, &title, &description, &priority, &status, &phase,
		&worktreePath, &baseBranch, &baseCommit, &assignee, &metaRaw, &wi.CreatedAt, &wi.UpdatedAt); err != nil {
		return nil, err
	}
	wi.ID = domain.WorkItemID(id)
	wi.Project = project
	wi.Title = title
	wi.Description = description
	wi.Priority = domain.Priority(priority)
	wi.Status = domain.Phase(status)
	wi.CurrentPhase = domain.Phase(phase)
	wi.WorktreePath = worktreePath
	wi.BaseBranch = baseBranch
	wi.BaseCommitSHA = baseCommit
	wi.Assignee = assignee
	wi.Metadata = domain.Metadata{}
	_ = unmarshal(metaRaw, &wi.Metadata)
	return &wi, nil
}
