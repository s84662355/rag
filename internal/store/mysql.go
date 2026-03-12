package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLStore struct {
	db *sql.DB
}

func NewMySQLStore(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return &MySQLStore{db: db}, nil
}

func (s *MySQLStore) Close() error {
	return s.db.Close()
}

func (s *MySQLStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *MySQLStore) CreateKnowledgeBase(ctx context.Context, name, description string) (*KnowledgeBase, error) {
	query := `
INSERT INTO knowledge_bases(name, description)
VALUES(?, ?)
`
	res, err := s.db.ExecContext(ctx, query, name, description)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge base: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.GetKnowledgeBase(ctx, id)
}

func (s *MySQLStore) GetKnowledgeBase(ctx context.Context, id int64) (*KnowledgeBase, error) {
	query := `
SELECT id, name, description, created_at, updated_at
FROM knowledge_bases
WHERE id = ?
`
	var kb KnowledgeBase
	if err := s.db.QueryRowContext(ctx, query, id).Scan(
		&kb.ID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("query knowledge base: %w", err)
	}
	return &kb, nil
}

func (s *MySQLStore) ListKnowledgeBases(ctx context.Context) ([]KnowledgeBase, error) {
	query := `
SELECT id, name, description, created_at, updated_at
FROM knowledge_bases
ORDER BY id DESC
`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	defer rows.Close()

	var out []KnowledgeBase
	for rows.Next() {
		var kb KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan knowledge bases: %w", err)
		}
		out = append(out, kb)
	}
	return out, rows.Err()
}

func (s *MySQLStore) DeleteKnowledgeBase(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM knowledge_bases WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete knowledge base: %w", err)
	}
	return nil
}

func (s *MySQLStore) CreateDocument(ctx context.Context, doc *Document) (int64, error) {
	query := `
INSERT INTO documents(kb_id, title, source_type, source_uri, raw_text, status, error_msg)
VALUES(?, ?, ?, ?, ?, ?, ?)
`
	res, err := s.db.ExecContext(
		ctx,
		query,
		doc.KBID,
		doc.Title,
		doc.SourceType,
		doc.SourceURI,
		doc.RawText,
		doc.Status,
		doc.ErrorMsg,
	)
	if err != nil {
		return 0, fmt.Errorf("insert document: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert document id: %w", err)
	}
	return id, nil
}

func (s *MySQLStore) UpdateDocumentStatus(ctx context.Context, id int64, status string, errMsg *string) error {
	query := `
UPDATE documents
SET status = ?, error_msg = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`
	if _, err := s.db.ExecContext(ctx, query, status, errMsg, id); err != nil {
		return fmt.Errorf("update document status: %w", err)
	}
	return nil
}

func (s *MySQLStore) ListDocumentsByKB(ctx context.Context, kbID int64) ([]Document, error) {
	query := `
SELECT id, kb_id, title, source_type, source_uri, status, error_msg, created_at, updated_at
FROM documents
WHERE kb_id = ?
ORDER BY id DESC
`
	rows, err := s.db.QueryContext(ctx, query, kbID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(
			&d.ID, &d.KBID, &d.Title, &d.SourceType, &d.SourceURI, &d.Status, &d.ErrorMsg, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan documents: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *MySQLStore) GetDocument(ctx context.Context, id int64) (*Document, error) {
	query := `
SELECT id, kb_id, title, source_type, source_uri, raw_text, status, error_msg, created_at, updated_at
FROM documents
WHERE id = ?
`
	var d Document
	if err := s.db.QueryRowContext(ctx, query, id).Scan(
		&d.ID, &d.KBID, &d.Title, &d.SourceType, &d.SourceURI, &d.RawText, &d.Status, &d.ErrorMsg, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	return &d, nil
}

func (s *MySQLStore) DeleteDocument(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func (s *MySQLStore) CreateChunks(ctx context.Context, chunks []Chunk) ([]int64, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx create chunks: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO chunks(doc_id, kb_id, chunk_index, content, token_count, metadata)
VALUES(?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return nil, fmt.Errorf("prepare chunk insert: %w", err)
	}
	defer stmt.Close()

	ids := make([]int64, 0, len(chunks))
	for _, c := range chunks {
		res, err := stmt.ExecContext(ctx, c.DocID, c.KBID, c.ChunkIndex, c.Content, c.TokenCount, c.Metadata)
		if err != nil {
			return nil, fmt.Errorf("insert chunk: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last insert chunk id: %w", err)
		}
		ids = append(ids, id)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit chunk insert: %w", err)
	}
	return ids, nil
}

func (s *MySQLStore) ListChunksByDoc(ctx context.Context, docID int64) ([]Chunk, error) {
	query := `
SELECT id, doc_id, kb_id, chunk_index, content, token_count, metadata, created_at, updated_at
FROM chunks
WHERE doc_id = ?
ORDER BY chunk_index ASC
`
	rows, err := s.db.QueryContext(ctx, query, docID)
	if err != nil {
		return nil, fmt.Errorf("list chunks: %w", err)
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(
			&c.ID, &c.DocID, &c.KBID, &c.ChunkIndex, &c.Content, &c.TokenCount, &c.Metadata, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chunks: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *MySQLStore) DeleteChunksByDoc(ctx context.Context, docID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM chunks WHERE doc_id = ?", docID)
	if err != nil {
		return fmt.Errorf("delete chunks by doc: %w", err)
	}
	return nil
}

func (s *MySQLStore) GetChunk(ctx context.Context, id int64) (*Chunk, error) {
	query := `
SELECT id, doc_id, kb_id, chunk_index, content, token_count, metadata, created_at, updated_at
FROM chunks
WHERE id = ?
`
	var c Chunk
	if err := s.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.DocID, &c.KBID, &c.ChunkIndex, &c.Content, &c.TokenCount, &c.Metadata, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}
	return &c, nil
}

func (s *MySQLStore) UpdateChunk(ctx context.Context, id int64, content string, tokenCount int) error {
	query := `
UPDATE chunks
SET content = ?, token_count = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`
	if _, err := s.db.ExecContext(ctx, query, content, tokenCount, id); err != nil {
		return fmt.Errorf("update chunk: %w", err)
	}
	return nil
}

func (s *MySQLStore) SaveQAPairs(ctx context.Context, pairs []QAPair) error {
	if len(pairs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx save qa: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO qa_pairs(doc_id, kb_id, question, answer, source_chunk_ids)
VALUES(?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare qa insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range pairs {
		if _, err := stmt.ExecContext(ctx, p.DocID, p.KBID, p.Question, p.Answer, p.SourceChunkIDs); err != nil {
			return fmt.Errorf("insert qa pair: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit qa insert: %w", err)
	}
	return nil
}

func (s *MySQLStore) ListQAPairsByDoc(ctx context.Context, docID int64) ([]QAPair, error) {
	query := `
SELECT id, doc_id, kb_id, question, answer, source_chunk_ids, created_at, updated_at
FROM qa_pairs
WHERE doc_id = ?
ORDER BY id DESC
`
	rows, err := s.db.QueryContext(ctx, query, docID)
	if err != nil {
		return nil, fmt.Errorf("list qa pairs: %w", err)
	}
	defer rows.Close()

	var out []QAPair
	for rows.Next() {
		var q QAPair
		if err := rows.Scan(&q.ID, &q.DocID, &q.KBID, &q.Question, &q.Answer, &q.SourceChunkIDs, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan qa pair: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *MySQLStore) DeleteQAPairsByDoc(ctx context.Context, docID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM qa_pairs WHERE doc_id = ?", docID)
	if err != nil {
		return fmt.Errorf("delete qa pairs by doc: %w", err)
	}
	return nil
}
