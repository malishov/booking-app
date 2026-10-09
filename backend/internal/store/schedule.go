package store

import "context"

// GetSchedule returns settings scoped to the owner, including an empty value
// for an existing business that has not configured its schedule yet.
func (q *Queries) GetSchedule(ctx context.Context, owner int64) ([]byte, error) {
	var settings []byte
	err := q.db.QueryRow(ctx, `SELECT COALESCE(s.settings, '{}'::jsonb) FROM businesses b
 LEFT JOIN master_schedules s ON s.business_id=b.id WHERE b.owner_user_id=$1`, owner).Scan(&settings)
	return settings, err
}

// SaveSchedule replaces the complete schedule atomically; business IDs from
// client input are never used.
func (q *Queries) SaveSchedule(ctx context.Context, owner int64, settings []byte) error {
	var id int64
	return q.db.QueryRow(ctx, `WITH owned AS (SELECT id FROM businesses WHERE owner_user_id=$1 FOR UPDATE)
 INSERT INTO master_schedules(business_id, settings)
 SELECT id, $2::jsonb FROM owned
 ON CONFLICT(business_id) DO UPDATE SET settings=EXCLUDED.settings, updated_at=now()
 RETURNING business_id`, owner, settings).Scan(&id)
}
