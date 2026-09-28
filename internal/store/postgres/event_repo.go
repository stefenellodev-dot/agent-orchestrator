package postgres

import (
	"context"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

func (s *Store) AppendEvent(ctx context.Context, e *domain.Event) error {
	payload := e.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO events (id, work_item_id, type, payload, actor, at)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6)`,
		string(e.ID), string(e.WorkItemID), string(e.Type), mustJSON(payload), string(e.Actor), e.At)
	return err
}

func (s *Store) ListEvents(ctx context.Context, workItemID domain.WorkItemID) ([]*domain.Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, work_item_id, type, payload, actor, at
		FROM events WHERE work_item_id=$1 ORDER BY at ASC`, string(workItemID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Event
	for rows.Next() {
		var (
			e          domain.Event
			id, wiID   string
			evType     string
			payloadRaw []byte
			actor      string
		)
		if err := rows.Scan(&id, &wiID, &evType, &payloadRaw, &actor, &e.At); err != nil {
			return nil, err
		}
		e.ID = domain.EventID(id)
		e.WorkItemID = domain.WorkItemID(wiID)
		e.Type = domain.EventType(evType)
		e.Actor = domain.EventActor(actor)
		e.Payload = map[string]any{}
		_ = unmarshal(payloadRaw, &e.Payload)
		out = append(out, &e)
	}
	return out, rows.Err()
}
