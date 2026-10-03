package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a row asked for by id does not exist.
var ErrNotFound = errors.New("not found")

const (
	TaskOpen      = "open"
	TaskDone      = "done"
	TaskCancelled = "cancelled"
)

// Task has no calendar slot: it lives only here, with a reminder time.
type Task struct {
	ID       int64
	Title    string
	Remind   time.Time
	Status   string
	Reminded bool
}

func (s *Store) CreateTask(ctx context.Context, title string, remind, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO tasks (title, remind_utc, created_utc) VALUES (?, ?, ?)",
		title, remind.Unix(), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("create task: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) Task(ctx context.Context, id int64) (Task, error) {
	tasks, err := s.queryTasks(ctx, "WHERE id = ?", id)
	if err != nil {
		return Task{}, err
	}
	if len(tasks) == 0 {
		return Task{}, fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	return tasks[0], nil
}

// DueTasks returns open tasks whose reminder time has come and that were not reminded yet.
func (s *Store) DueTasks(ctx context.Context, now time.Time) ([]Task, error) {
	return s.queryTasks(ctx, "WHERE status = 'open' AND reminded = 0 AND remind_utc <= ? ORDER BY remind_utc, id", now.Unix())
}

// OpenTasksBefore returns open tasks due before t, overdue ones included.
func (s *Store) OpenTasksBefore(ctx context.Context, t time.Time) ([]Task, error) {
	return s.queryTasks(ctx, "WHERE status = 'open' AND remind_utc < ? ORDER BY remind_utc, id", t.Unix())
}

func (s *Store) MarkTaskReminded(ctx context.Context, id int64) error {
	return s.updateTask(ctx, id, "UPDATE tasks SET reminded = 1 WHERE id = ?", id)
}

func (s *Store) SetTaskStatus(ctx context.Context, id int64, status string) error {
	return s.updateTask(ctx, id, "UPDATE tasks SET status = ? WHERE id = ?", status, id)
}

// RescheduleTask moves the reminder and arms it again.
func (s *Store) RescheduleTask(ctx context.Context, id int64, remind time.Time) error {
	return s.updateTask(ctx, id, "UPDATE tasks SET remind_utc = ?, reminded = 0 WHERE id = ?", remind.Unix(), id)
}

func (s *Store) updateTask(ctx context.Context, id int64, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update task %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Store) queryTasks(ctx context.Context, where string, args ...any) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, title, remind_utc, status, reminded FROM tasks "+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var (
			t      Task
			remind int64
		)
		if err := rows.Scan(&t.ID, &t.Title, &remind, &t.Status, &t.Reminded); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		t.Remind = time.Unix(remind, 0).UTC()
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	return tasks, nil
}
