package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrQuestionNotFound = errors.New("question not found")
	ErrReplyNotFound    = errors.New("reply not found")
)

type Question struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	UserID    int64     `json:"user_id"`
	Question  string    `json:"question"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Reply struct {
	ID         int64     `json:"id"`
	QuestionID int64     `json:"question_id"`
	AdminID    int64     `json:"admin_id"`
	Reply      string    `json:"reply"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type QuestionWithReplies struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	CreatedAt time.Time `json:"created_at"`
	Replies   []Reply   `json:"replies,omitempty"`
}

type QuestionStore struct {
	db *sql.DB
}

// CreateQuestion creates a new game question
func (s *QuestionStore) CreateQuestion(ctx context.Context, question *Question) error {
	query := `
		INSERT INTO game_questions (game_id, user_id, question)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRowContext(ctx, query,
		question.GameID,
		question.UserID,
		question.Question,
	).Scan(
		&question.ID,
		&question.CreatedAt,
		&question.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("error creating question: %w", err)
	}

	return nil
}

// GetQuestionsByGame returns all questions for a game
func (s *QuestionStore) GetQuestionsByGame(ctx context.Context, gameID int64) ([]Question, error) {
	query := `
		SELECT id, game_id, user_id, question, created_at, updated_at
		FROM game_questions
		WHERE game_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("error fetching questions: %w", err)
	}
	defer rows.Close()

	var questions []Question
	for rows.Next() {
		var q Question
		if err := rows.Scan(
			&q.ID,
			&q.GameID,
			&q.UserID,
			&q.Question,
			&q.CreatedAt,
			&q.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning question: %w", err)
		}
		questions = append(questions, q)
	}

	return questions, nil
}

// CreateReply creates a reply to a question
func (s *QuestionStore) CreateReply(ctx context.Context, reply *Reply) error {
	query := `
		INSERT INTO game_question_replies (question_id, admin_id, reply)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRowContext(ctx, query,
		reply.QuestionID,
		reply.AdminID,
		reply.Reply,
	).Scan(
		&reply.ID,
		&reply.CreatedAt,
		&reply.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("error creating reply: %w", err)
	}

	return nil
}

// GetRepliesByQuestion returns all replies for a question
func (s *QuestionStore) GetRepliesByQuestion(ctx context.Context, questionID int64) ([]Reply, error) {
	query := `
		SELECT id, question_id, admin_id, reply, created_at, updated_at
		FROM game_question_replies
		WHERE question_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, questionID)
	if err != nil {
		return nil, fmt.Errorf("error fetching replies: %w", err)
	}
	defer rows.Close()

	var replies []Reply
	for rows.Next() {
		var r Reply
		if err := rows.Scan(
			&r.ID,
			&r.QuestionID,
			&r.AdminID,
			&r.Reply,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning reply: %w", err)
		}
		replies = append(replies, r)
	}

	return replies, nil
}

// DeleteQuestion soft deletes a question
func (s *QuestionStore) DeleteQuestion(ctx context.Context, questionID, userID int64) error {
	query := `
		DELETE FROM game_questions
		WHERE id = $1 AND user_id = $2
	`

	result, err := s.db.ExecContext(ctx, query, questionID, userID)
	if err != nil {
		return fmt.Errorf("error deleting question: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrQuestionNotFound
	}

	return nil
}

func (s *QuestionStore) GetQuestionsWithReplies(ctx context.Context, gameID int64) ([]QuestionWithReplies, error) {
	query := `
		SELECT 
			q.id AS question_id,
			q.question,
			q.created_at,
			r.id AS reply_id,
			r.admin_id,
			r.reply,
			r.created_at AS reply_created
		FROM game_questions q
		LEFT JOIN game_question_replies r ON q.id = r.question_id
		WHERE q.game_id = $1
		ORDER BY q.created_at DESC, r.created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("error fetching questions with replies: %w", err)
	}
	defer rows.Close()

	questionsMap := make(map[int64]*QuestionWithReplies)
	for rows.Next() {
		var (
			qID      int64
			question string
			qCreated time.Time
			rID      sql.NullInt64
			adminID  sql.NullInt64
			reply    sql.NullString
			rCreated sql.NullTime
		)

		if err := rows.Scan(
			&qID,
			&question,
			&qCreated,
			&rID,
			&adminID,
			&reply,
			&rCreated,
		); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		// Create question entry if not exists
		if _, exists := questionsMap[qID]; !exists {
			questionsMap[qID] = &QuestionWithReplies{
				ID:        qID,
				Question:  question,
				CreatedAt: qCreated,
			}
		}

		// Add reply if exists
		if rID.Valid && reply.Valid {
			questionsMap[qID].Replies = append(questionsMap[qID].Replies, Reply{
				ID:        rID.Int64,
				AdminID:   adminID.Int64,
				Reply:     reply.String,
				CreatedAt: rCreated.Time,
			})
		}
	}

	// Convert map to slice
	result := make([]QuestionWithReplies, 0, len(questionsMap))
	for _, q := range questionsMap {
		result = append(result, *q)
	}

	return result, nil
}
