package paymentsrepo

import (
	"context"
	"encoding/json"
	"fmt"

	"khel/internal/infra/dbx"
)

type LogsRepository struct{ q dbx.Querier }

func NewLogsRepository(q dbx.Querier) *LogsRepository {
	return &LogsRepository{q: q}
}

func (r *LogsRepository) InsertPaymentLog(ctx context.Context, paymentID int64, logType string, payload any) error {
	var jb []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err == nil {
			jb = b
		}
	}

	_, err := r.q.Exec(ctx, `
		INSERT INTO payment_logs (payment_id, log_type, payload)
		VALUES ($1, $2, $3)
	`, paymentID, logType, jb)
	if err != nil {
		return fmt.Errorf("insert payment_log: %w", err)
	}
	return nil
}
