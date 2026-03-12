package store

import "time"

type KnowledgeBase struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Document struct {
	ID         int64     `json:"id"`
	KBID       int64     `json:"kb_id"`
	Title      string    `json:"title"`
	SourceType string    `json:"source_type"`
	SourceURI  string    `json:"source_uri"`
	RawText    string    `json:"raw_text,omitempty"`
	Status     string    `json:"status"`
	ErrorMsg   *string   `json:"error_msg,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Chunk struct {
	ID         int64     `json:"id"`
	DocID      int64     `json:"doc_id"`
	KBID       int64     `json:"kb_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	TokenCount int       `json:"token_count"`
	Metadata   *string   `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type QAPair struct {
	ID             int64     `json:"id"`
	DocID          int64     `json:"doc_id"`
	KBID           int64     `json:"kb_id"`
	Question       string    `json:"question"`
	Answer         string    `json:"answer"`
	SourceChunkIDs string    `json:"source_chunk_ids"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
