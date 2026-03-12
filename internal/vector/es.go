package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type ESClient struct {
	address   string
	indexName string
	dims      int
	http      *http.Client
}

type IndexedChunk struct {
	ChunkID int64     `json:"chunk_id"`
	DocID   int64     `json:"doc_id"`
	KBID    int64     `json:"kb_id"`
	Content string    `json:"content"`
	Vector  []float64 `json:"content_vector"`
}

type SearchHit struct {
	ChunkID      int64   `json:"chunk_id"`
	DocID        int64   `json:"doc_id"`
	KBID         int64   `json:"kb_id"`
	Content      string  `json:"content"`
	Score        float64 `json:"score"`
	DenseScore   float64 `json:"dense_score,omitempty"`
	KeywordScore float64 `json:"keyword_score,omitempty"`
	Source       string  `json:"source"`
}

type fuseAgg struct {
	hit      SearchHit
	rrfScore float64
}

func NewESClient(address, indexName string, dims int) *ESClient {
	return &ESClient{
		address:   strings.TrimRight(address, "/"),
		indexName: indexName,
		dims:      dims,
		http: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *ESClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.address, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("es ping status=%d", resp.StatusCode)
	}
	return nil
}

func (c *ESClient) EnsureIndex(ctx context.Context) error {
	mapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
		},
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"chunk_id": map[string]interface{}{"type": "long"},
				"doc_id":   map[string]interface{}{"type": "long"},
				"kb_id":    map[string]interface{}{"type": "long"},
				"content": map[string]interface{}{
					"type":     "text",
					"analyzer": "standard",
				},
				"content_vector": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       c.dims,
					"index":      true,
					"similarity": "cosine",
				},
			},
		},
	}
	body, _ := json.Marshal(mapping)
	url := fmt.Sprintf("%s/%s", c.address, c.indexName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ensure-index request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call ensure-index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 {
		return c.ensureIndexSettings(ctx)
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == 400 && bytes.Contains(respBody, []byte("resource_already_exists_exception")) {
		return c.ensureIndexSettings(ctx)
	}
	return fmt.Errorf("ensure index failed status=%d body=%s", resp.StatusCode, string(respBody))
}

func (c *ESClient) ensureIndexSettings(ctx context.Context) error {
	reqBody := map[string]interface{}{
		"index": map[string]interface{}{
			"number_of_replicas": 0,
		},
	}
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/%s/_settings", c.address, c.indexName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ensure-index-settings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call ensure-index-settings: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ensure index settings failed status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *ESClient) UpsertChunk(ctx context.Context, chunk IndexedChunk) error {
	body, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("marshal chunk: %w", err)
	}
	url := fmt.Sprintf("%s/%s/_doc/%d", c.address, c.indexName, chunk.ChunkID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create upsert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upsert chunk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upsert chunk status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *ESClient) DeleteByDoc(ctx context.Context, docID int64) error {
	reqBody := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"doc_id": docID,
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/%s/_delete_by_query", c.address, c.indexName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create delete by doc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete by doc: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("delete by doc status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *ESClient) SearchDense(ctx context.Context, kbID int64, queryVector []float64, topK int) ([]SearchHit, error) {
	if topK <= 0 {
		topK = 8
	}
	reqBody := map[string]interface{}{
		"size": topK,
		"query": map[string]interface{}{
			"script_score": map[string]interface{}{
				"query": map[string]interface{}{
					"bool": map[string]interface{}{
						"filter": []map[string]interface{}{
							{"term": map[string]interface{}{"kb_id": kbID}},
						},
					},
				},
				"script": map[string]interface{}{
					"source": "cosineSimilarity(params.qv, 'content_vector') + 1.0",
					"params": map[string]interface{}{
						"qv": queryVector,
					},
				},
			},
		},
		"_source": []string{"chunk_id", "doc_id", "kb_id", "content"},
	}
	return c.search(ctx, reqBody, "dense")
}

func (c *ESClient) SearchKeyword(ctx context.Context, kbID int64, query string, topK int) ([]SearchHit, error) {
	if topK <= 0 {
		topK = 8
	}
	reqBody := map[string]interface{}{
		"size": topK,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"match": map[string]interface{}{"content": query}},
				},
				"filter": []map[string]interface{}{
					{"term": map[string]interface{}{"kb_id": kbID}},
				},
			},
		},
		"_source": []string{"chunk_id", "doc_id", "kb_id", "content"},
	}
	return c.search(ctx, reqBody, "keyword")
}

func (c *ESClient) search(ctx context.Context, reqBody map[string]interface{}, source string) ([]SearchHit, error) {
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/%s/_search", c.address, c.indexName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", source, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("search %s status=%d body=%s", source, resp.StatusCode, string(b))
	}

	var raw struct {
		Hits struct {
			Hits []struct {
				Score  float64 `json:"_score"`
				Source struct {
					ChunkID int64  `json:"chunk_id"`
					DocID   int64  `json:"doc_id"`
					KBID    int64  `json:"kb_id"`
					Content string `json:"content"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	out := make([]SearchHit, 0, len(raw.Hits.Hits))
	for _, h := range raw.Hits.Hits {
		hit := SearchHit{
			ChunkID: h.Source.ChunkID,
			DocID:   h.Source.DocID,
			KBID:    h.Source.KBID,
			Content: h.Source.Content,
			Score:   h.Score,
			Source:  source,
		}
		if source == "dense" {
			hit.DenseScore = h.Score
		}
		if source == "keyword" {
			hit.KeywordScore = h.Score
		}
		out = append(out, hit)
	}
	return out, nil
}

func FuseByRRF(dense, keyword []SearchHit, topN int) []SearchHit {
	if topN <= 0 {
		topN = 8
	}
	m := make(map[int64]*fuseAgg, len(dense)+len(keyword))
	const k = 60.0

	for i, d := range dense {
		a := ensureAgg(m, d)
		a.rrfScore += 1.0 / (k + float64(i+1))
		a.hit.DenseScore = d.Score
		a.hit.Source = mergeSource(a.hit.Source, "dense")
	}
	for i, kw := range keyword {
		a := ensureAgg(m, kw)
		a.rrfScore += 1.0 / (k + float64(i+1))
		a.hit.KeywordScore = kw.Score
		a.hit.Source = mergeSource(a.hit.Source, "keyword")
	}

	out := make([]SearchHit, 0, len(m))
	for _, a := range m {
		a.hit.Score = a.rrfScore
		out = append(out, a.hit)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if len(out) > topN {
		out = out[:topN]
	}
	return out
}

func ensureAgg(m map[int64]*fuseAgg, hit SearchHit) *fuseAgg {
	if v, ok := m[hit.ChunkID]; ok {
		return v
	}
	v := &fuseAgg{hit: hit}
	m[hit.ChunkID] = v
	return v
}

func mergeSource(base, ext string) string {
	if base == "" {
		return ext
	}
	if strings.Contains(base, ext) {
		return base
	}
	return base + "+" + ext
}
