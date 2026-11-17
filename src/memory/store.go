package memory

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Message struct {
	Role    string
	Content string
}

type Note struct {
	ID        int64
	Project   string
	Text      string
	CreatedAt time.Time
}

func Open(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	databasePath := filepath.Join(dataDir, "assistant.db")
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("create memory database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close memory database file: %w", err)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open memory database: %w", err)
	}
	db.SetMaxOpenConns(1)

	for _, statement := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS messages_chat_id_id ON messages(chat_id, id)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			project_slug TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS notes_chat_project_id ON notes(chat_id, project_slug, id)`,
		`CREATE TABLE IF NOT EXISTS chat_settings (
			chat_id INTEGER PRIMARY KEY,
			project_slug TEXT NOT NULL DEFAULT ''
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("initialize memory database: %w", err)
		}
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveMessage(chatID int64, role, content string) error {
	if role != "user" && role != "assistant" {
		return fmt.Errorf("unsupported message role %q", role)
	}
	_, err := s.db.Exec(
		"INSERT INTO messages (chat_id, role, content, created_at) VALUES (?, ?, ?, ?)",
		chatID, role, content, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save conversation message: %w", err)
	}
	return nil
}

func (s *Store) History(chatID int64, limit int) ([]Message, error) {
	if limit <= 0 {
		return nil, errors.New("history limit must be positive")
	}
	rows, err := s.db.Query(
		`SELECT role, content FROM (
			SELECT id, role, content FROM messages WHERE chat_id = ? ORDER BY id DESC LIMIT ?
		) ORDER BY id ASC`,
		chatID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("load conversation history: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			return nil, fmt.Errorf("read conversation history: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read conversation history: %w", err)
	}
	return messages, nil
}

func (s *Store) ResetConversation(chatID int64) error {
	if _, err := s.db.Exec("DELETE FROM messages WHERE chat_id = ?", chatID); err != nil {
		return fmt.Errorf("reset conversation: %w", err)
	}
	return nil
}

func (s *Store) SetActiveProject(chatID int64, project string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_settings (chat_id, project_slug) VALUES (?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET project_slug = excluded.project_slug`,
		chatID, project,
	)
	if err != nil {
		return fmt.Errorf("save active project: %w", err)
	}
	return nil
}

func (s *Store) ActiveProject(chatID int64) (string, error) {
	var project string
	err := s.db.QueryRow("SELECT project_slug FROM chat_settings WHERE chat_id = ?", chatID).Scan(&project)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load active project: %w", err)
	}
	return project, nil
}

func (s *Store) AddNote(chatID int64, project, text string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO notes (chat_id, project_slug, content, created_at) VALUES (?, ?, ?, ?)",
		chatID, project, text, time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("save note: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get note ID: %w", err)
	}
	return id, nil
}

func (s *Store) Notes(chatID int64, project string, limit int) ([]Note, error) {
	if limit <= 0 {
		return nil, errors.New("note limit must be positive")
	}
	rows, err := s.db.Query(
		`SELECT id, project_slug, content, created_at FROM notes
		WHERE chat_id = ? AND (project_slug = '' OR project_slug = ?)
		ORDER BY id DESC LIMIT ?`,
		chatID, project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var note Note
		var createdAt int64
		if err := rows.Scan(&note.ID, &note.Project, &note.Text, &createdAt); err != nil {
			return nil, fmt.Errorf("read notes: %w", err)
		}
		note.CreatedAt = time.Unix(createdAt, 0)
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read notes: %w", err)
	}
	return notes, nil
}

func (s *Store) ForgetNote(chatID, noteID int64) (bool, error) {
	result, err := s.db.Exec("DELETE FROM notes WHERE chat_id = ? AND id = ?", chatID, noteID)
	if err != nil {
		return false, fmt.Errorf("forget note: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check forgotten note: %w", err)
	}
	return count > 0, nil
}
