package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"rag/internal/chunker"
	"rag/internal/embedding"
	"rag/internal/llm"
	"rag/internal/parser"
	"rag/internal/store"
	"rag/internal/vector"
)

type RAGService struct {
	store   *store.MySQLStore
	vector  *vector.ESClient
	embed   *embedding.Client
	rewrite *llm.Client
	rerank  *llm.Client
	qa      *llm.Client
	chat    *llm.Client

	chunkMaxChars int
	chunkOverlap  int

	mu       sync.Mutex
	sessions map[string][]Turn
}

type Turn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SearchRequest struct {
	KBID       int64  `json:"kb_id"`
	Query      string `json:"query"`
	TopK       int    `json:"top_k"`
	UseRewrite bool   `json:"use_rewrite"`
	UseRerank  bool   `json:"use_rerank"`
}

type SearchResponse struct {
	OriginalQuery  string             `json:"original_query"`
	RewrittenQuery string             `json:"rewritten_query"`
	Hits           []vector.SearchHit `json:"hits"`
	DenseHits      []vector.SearchHit `json:"dense_hits,omitempty"`
	KeywordHits    []vector.SearchHit `json:"keyword_hits,omitempty"`
}

type URLIngestRequest struct {
	KBID int64  `json:"kb_id"`
	URL  string `json:"url"`
}

type QAGenerateRequest struct {
	DocID int64 `json:"doc_id"`
	Limit int   `json:"limit"`
}

type ChatRequest struct {
	KBID      int64  `json:"kb_id"`
	SessionID string `json:"session_id"`
	Question  string `json:"question"`
	TopK      int    `json:"top_k"`
}

type ChatResponse struct {
	SessionID      string             `json:"session_id"`
	Question       string             `json:"question"`
	RewrittenQuery string             `json:"rewritten_query"`
	Answer         string             `json:"answer"`
	References     []vector.SearchHit `json:"references"`
}

type ReindexDocumentResponse struct {
	DocumentID int64 `json:"document_id"`
	ChunkCount int   `json:"chunk_count"`
}

func NewRAGService(
	store *store.MySQLStore,
	vectorClient *vector.ESClient,
	embedClient *embedding.Client,
	rewriteLLM, rerankLLM, qaLLM, chatLLM *llm.Client,
	chunkMaxChars, chunkOverlap int,
) *RAGService {
	return &RAGService{
		store:         store,
		vector:        vectorClient,
		embed:         embedClient,
		rewrite:       rewriteLLM,
		rerank:        rerankLLM,
		qa:            qaLLM,
		chat:          chatLLM,
		chunkMaxChars: chunkMaxChars,
		chunkOverlap:  chunkOverlap,
		sessions:      make(map[string][]Turn),
	}
}

func (s *RAGService) Health(ctx context.Context) (map[string]string, error) {
	result := map[string]string{
		"mysql": "ok",
		"es":    "ok",
	}
	if err := s.store.Ping(ctx); err != nil {
		result["mysql"] = err.Error()
	}
	if err := s.vector.Ping(ctx); err != nil {
		result["es"] = err.Error()
	}
	return result, nil
}

func (s *RAGService) CreateKnowledgeBase(ctx context.Context, name, description string) (*store.KnowledgeBase, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	return s.store.CreateKnowledgeBase(ctx, strings.TrimSpace(name), strings.TrimSpace(description))
}

func (s *RAGService) ListKnowledgeBases(ctx context.Context) ([]store.KnowledgeBase, error) {
	return s.store.ListKnowledgeBases(ctx)
}

func (s *RAGService) DeleteKnowledgeBase(ctx context.Context, id int64) error {
	return s.store.DeleteKnowledgeBase(ctx, id)
}

func (s *RAGService) IngestFile(ctx context.Context, kbID int64, filename string, data []byte) (*store.Document, int, error) {
	parsed, err := parser.ParseByFilename(filename, data)
	if err != nil {
		return nil, 0, err
	}
	return s.ingestParsedDocument(ctx, kbID, parsed.Title, "file", filename, parsed.Text)
}

func (s *RAGService) IngestURL(ctx context.Context, req URLIngestRequest) (*store.Document, int, error) {
	parsed, err := parser.ParseWebPage(req.URL)
	if err != nil {
		return nil, 0, err
	}
	return s.ingestParsedDocument(ctx, req.KBID, parsed.Title, "url", req.URL, parsed.Text)
}

func (s *RAGService) ingestParsedDocument(ctx context.Context, kbID int64, title, sourceType, sourceURI, text string) (*store.Document, int, error) {
	if kbID <= 0 {
		return nil, 0, fmt.Errorf("kb_id must be greater than zero")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, 0, fmt.Errorf("document text is empty after parsing")
	}

	doc := &store.Document{
		KBID:       kbID,
		Title:      strings.TrimSpace(title),
		SourceType: sourceType,
		SourceURI:  sourceURI,
		RawText:    text,
		Status:     "processing",
	}
	docID, err := s.store.CreateDocument(ctx, doc)
	if err != nil {
		return nil, 0, err
	}
	doc.ID = docID

	chunksText := chunker.SplitText(text, s.chunkMaxChars, s.chunkOverlap)
	if len(chunksText) == 0 {
		msg := "no chunks generated"
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, 0, fmt.Errorf(msg)
	}

	chunks := make([]store.Chunk, 0, len(chunksText))
	for i, c := range chunksText {
		chunks = append(chunks, store.Chunk{
			DocID:      docID,
			KBID:       kbID,
			ChunkIndex: i,
			Content:    c,
			TokenCount: chunker.EstimateTokens(c),
		})
	}
	chunkIDs, err := s.store.CreateChunks(ctx, chunks)
	if err != nil {
		msg := err.Error()
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, 0, err
	}

	for i, chunkID := range chunkIDs {
		vec, err := s.embed.Embed(ctx, chunksText[i])
		if err != nil {
			msg := fmt.Sprintf("embedding chunk %d: %v", chunkID, err)
			_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
			return nil, len(chunksText), err
		}
		if err := s.vector.UpsertChunk(ctx, vector.IndexedChunk{
			ChunkID: chunkID,
			DocID:   docID,
			KBID:    kbID,
			Content: chunksText[i],
			Vector:  vec,
		}); err != nil {
			msg := fmt.Sprintf("index chunk %d: %v", chunkID, err)
			_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
			return nil, len(chunksText), err
		}
	}

	if err := s.store.UpdateDocumentStatus(ctx, docID, "ready", nil); err != nil {
		return nil, len(chunksText), err
	}
	doc.Status = "ready"
	return doc, len(chunksText), nil
}

func (s *RAGService) ListDocuments(ctx context.Context, kbID int64) ([]store.Document, error) {
	return s.store.ListDocumentsByKB(ctx, kbID)
}

func (s *RAGService) DeleteDocument(ctx context.Context, docID int64) error {
	if docID <= 0 {
		return fmt.Errorf("doc_id must be greater than zero")
	}
	if _, err := s.store.GetDocument(ctx, docID); err != nil {
		return err
	}

	if err := s.vector.DeleteByDoc(ctx, docID); err != nil {
		return err
	}
	return s.store.DeleteDocument(ctx, docID)
}

func (s *RAGService) ReindexDocument(ctx context.Context, docID int64) (*ReindexDocumentResponse, error) {
	if docID <= 0 {
		return nil, fmt.Errorf("doc_id must be greater than zero")
	}
	doc, err := s.store.GetDocument(ctx, docID)
	if err != nil {
		return nil, err
	}
	rawText := strings.TrimSpace(doc.RawText)
	if rawText == "" {
		return nil, fmt.Errorf("document raw_text is empty, cannot reindex")
	}

	if err := s.store.UpdateDocumentStatus(ctx, docID, "processing", nil); err != nil {
		return nil, err
	}

	if err := s.vector.DeleteByDoc(ctx, docID); err != nil {
		msg := fmt.Sprintf("delete old vectors failed: %v", err)
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, err
	}
	if err := s.store.DeleteChunksByDoc(ctx, docID); err != nil {
		msg := fmt.Sprintf("delete old chunks failed: %v", err)
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, err
	}
	if err := s.store.DeleteQAPairsByDoc(ctx, docID); err != nil {
		msg := fmt.Sprintf("delete old qa pairs failed: %v", err)
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, err
	}

	chunksText := chunker.SplitText(rawText, s.chunkMaxChars, s.chunkOverlap)
	if len(chunksText) == 0 {
		msg := "no chunks generated during reindex"
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, fmt.Errorf(msg)
	}

	chunks := make([]store.Chunk, 0, len(chunksText))
	for i, c := range chunksText {
		chunks = append(chunks, store.Chunk{
			DocID:      docID,
			KBID:       doc.KBID,
			ChunkIndex: i,
			Content:    c,
			TokenCount: chunker.EstimateTokens(c),
		})
	}

	chunkIDs, err := s.store.CreateChunks(ctx, chunks)
	if err != nil {
		msg := fmt.Sprintf("create chunks failed: %v", err)
		_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
		return nil, err
	}

	for i, chunkID := range chunkIDs {
		vec, err := s.embed.Embed(ctx, chunksText[i])
		if err != nil {
			msg := fmt.Sprintf("embedding chunk %d failed: %v", chunkID, err)
			_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
			return nil, err
		}
		if err := s.vector.UpsertChunk(ctx, vector.IndexedChunk{
			ChunkID: chunkID,
			DocID:   docID,
			KBID:    doc.KBID,
			Content: chunksText[i],
			Vector:  vec,
		}); err != nil {
			msg := fmt.Sprintf("index chunk %d failed: %v", chunkID, err)
			_ = s.store.UpdateDocumentStatus(ctx, docID, "failed", &msg)
			return nil, err
		}
	}

	if err := s.store.UpdateDocumentStatus(ctx, docID, "ready", nil); err != nil {
		return nil, err
	}
	return &ReindexDocumentResponse{
		DocumentID: docID,
		ChunkCount: len(chunksText),
	}, nil
}

func (s *RAGService) ListChunksByDoc(ctx context.Context, docID int64) ([]store.Chunk, error) {
	return s.store.ListChunksByDoc(ctx, docID)
}

func (s *RAGService) UpdateChunk(ctx context.Context, chunkID int64, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("chunk content cannot be empty")
	}
	ch, err := s.store.GetChunk(ctx, chunkID)
	if err != nil {
		return err
	}
	if err := s.store.UpdateChunk(ctx, chunkID, content, chunker.EstimateTokens(content)); err != nil {
		return err
	}

	vec, err := s.embed.Embed(ctx, content)
	if err != nil {
		return err
	}
	return s.vector.UpsertChunk(ctx, vector.IndexedChunk{
		ChunkID: chunkID,
		DocID:   ch.DocID,
		KBID:    ch.KBID,
		Content: content,
		Vector:  vec,
	})
}

func (s *RAGService) Search(ctx context.Context, req SearchRequest, includeDebug bool) (*SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if req.KBID <= 0 {
		return nil, fmt.Errorf("kb_id must be greater than zero")
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}

	rewritten := query
	if req.UseRewrite {
		if q, err := s.rewriteQuery(ctx, query); err == nil && q != "" {
			rewritten = q
		}
	}

	queryVector, err := s.embed.Embed(ctx, rewritten)
	if err != nil {
		return nil, err
	}

	var denseHits []vector.SearchHit
	var keywordHits []vector.SearchHit
	var denseErr, keywordErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		denseHits, denseErr = s.vector.SearchDense(ctx, req.KBID, queryVector, req.TopK*3)
	}()
	go func() {
		defer wg.Done()
		keywordHits, keywordErr = s.vector.SearchKeyword(ctx, req.KBID, rewritten, req.TopK*3)
	}()
	wg.Wait()

	if denseErr != nil && keywordErr != nil {
		return nil, fmt.Errorf("dense search error: %v; keyword search error: %v", denseErr, keywordErr)
	}

	fused := vector.FuseByRRF(denseHits, keywordHits, req.TopK*3)
	if req.UseRerank && len(fused) > 1 {
		if reranked, err := s.rerankHits(ctx, rewritten, fused); err == nil && len(reranked) > 0 {
			fused = reranked
		}
	}
	if len(fused) > req.TopK {
		fused = fused[:req.TopK]
	}

	resp := &SearchResponse{
		OriginalQuery:  query,
		RewrittenQuery: rewritten,
		Hits:           fused,
	}
	if includeDebug {
		resp.DenseHits = denseHits
		resp.KeywordHits = keywordHits
	}
	return resp, nil
}

func (s *RAGService) rewriteQuery(ctx context.Context, query string) (string, error) {
	if s.rewrite == nil || !s.rewrite.Available() {
		return query, nil
	}
	system := "你是检索优化助手。仅输出改写后的单行中文问题，不要额外解释。"
	user := "请改写以下问题用于知识库检索，保持原意更准确：\n" + query
	out, err := s.rewrite.Chat(ctx, system, user, 0.1)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(strings.Split(out, "\n")[0])
	if out == "" {
		return query, nil
	}
	return out, nil
}

func (s *RAGService) rerankHits(ctx context.Context, query string, hits []vector.SearchHit) ([]vector.SearchHit, error) {
	if s.rerank == nil || !s.rerank.Available() {
		return hits, nil
	}

	maxCandidates := 15
	if len(hits) < maxCandidates {
		maxCandidates = len(hits)
	}
	candidates := hits[:maxCandidates]

	var b strings.Builder
	for _, h := range candidates {
		content := h.Content
		if len([]rune(content)) > 260 {
			content = string([]rune(content)[:260])
		}
		b.WriteString(fmt.Sprintf("id=%d text=%s\n", h.ChunkID, strings.ReplaceAll(content, "\n", " ")))
	}

	system := "你是结果重排序器。只输出JSON数组，数组元素是chunk id（整数），按相关度从高到低排序。"
	user := fmt.Sprintf("问题：%s\n候选文档：\n%s", query, b.String())
	out, err := s.rerank.Chat(ctx, system, user, 0.0)
	if err != nil {
		return nil, err
	}

	ids := parseRerankIDs(out)
	if len(ids) == 0 {
		return nil, fmt.Errorf("rerank output parse failed")
	}

	idPos := make(map[int64]int, len(ids))
	for i, id := range ids {
		idPos[id] = i
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		pi, okI := idPos[candidates[i].ChunkID]
		pj, okJ := idPos[candidates[j].ChunkID]
		switch {
		case okI && okJ:
			return pi < pj
		case okI:
			return true
		case okJ:
			return false
		default:
			return candidates[i].Score > candidates[j].Score
		}
	})
	return append(candidates, hits[maxCandidates:]...), nil
}

func (s *RAGService) GenerateQAPairs(ctx context.Context, req QAGenerateRequest) (int, error) {
	if s.qa == nil || !s.qa.Available() {
		return 0, fmt.Errorf("qa llm is not configured")
	}
	chunks, err := s.store.ListChunksByDoc(ctx, req.DocID)
	if err != nil {
		return 0, err
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	if req.Limit <= 0 || req.Limit > len(chunks) {
		req.Limit = len(chunks)
	}

	doc, err := s.store.GetDocument(ctx, req.DocID)
	if err != nil {
		return 0, err
	}

	var pairs []store.QAPair
	for i := 0; i < req.Limit; i++ {
		ch := chunks[i]
		system := "你是问答数据生成器。只输出JSON数组，每项包含question和answer字段。"
		user := fmt.Sprintf("请根据以下知识片段生成2个高质量问答对。\n知识片段：\n%s", ch.Content)
		out, err := s.qa.Chat(ctx, system, user, 0.2)
		if err != nil {
			continue
		}

		items := parseQAPairs(out)
		for _, item := range items {
			item.Question = strings.TrimSpace(item.Question)
			item.Answer = strings.TrimSpace(item.Answer)
			if item.Question == "" || item.Answer == "" {
				continue
			}
			src, _ := json.Marshal([]int64{ch.ID})
			pairs = append(pairs, store.QAPair{
				DocID:          req.DocID,
				KBID:           doc.KBID,
				Question:       item.Question,
				Answer:         item.Answer,
				SourceChunkIDs: string(src),
			})
		}
	}
	if len(pairs) == 0 {
		return 0, nil
	}
	if err := s.store.SaveQAPairs(ctx, pairs); err != nil {
		return 0, err
	}
	return len(pairs), nil
}

func (s *RAGService) ListQAPairs(ctx context.Context, docID int64) ([]store.QAPair, error) {
	return s.store.ListQAPairsByDoc(ctx, docID)
}

func (s *RAGService) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if s.chat == nil || !s.chat.Available() {
		return nil, fmt.Errorf("chat llm is not configured")
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	retrieved, err := s.Search(ctx, SearchRequest{
		KBID:       req.KBID,
		Query:      req.Question,
		TopK:       req.TopK,
		UseRewrite: true,
		UseRerank:  true,
	}, false)
	if err != nil {
		return nil, err
	}

	var contextBuilder strings.Builder
	for _, hit := range retrieved.Hits {
		contextBuilder.WriteString(fmt.Sprintf("[chunk:%d]\n%s\n\n", hit.ChunkID, hit.Content))
	}

	history := s.loadHistory(req.SessionID)
	system := "你是企业知识库问答助手。回答必须严格基于给定知识片段，未知则明确说不知道。回答结尾给出引用chunk id，例如[chunk:12]。"
	user := fmt.Sprintf("历史对话：\n%s\n\n知识片段：\n%s\n当前问题：%s", history, contextBuilder.String(), req.Question)

	answer, err := s.chat.Chat(ctx, system, user, 0.2)
	if err != nil {
		return nil, err
	}
	s.appendHistory(req.SessionID, Turn{Role: "user", Content: req.Question}, Turn{Role: "assistant", Content: answer})

	return &ChatResponse{
		SessionID:      req.SessionID,
		Question:       req.Question,
		RewrittenQuery: retrieved.RewrittenQuery,
		Answer:         strings.TrimSpace(answer),
		References:     retrieved.Hits,
	}, nil
}

func (s *RAGService) loadHistory(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return "(无)"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	turns := s.sessions[sessionID]
	if len(turns) == 0 {
		return "(无)"
	}

	start := 0
	if len(turns) > 8 {
		start = len(turns) - 8
	}
	var b strings.Builder
	for _, t := range turns[start:] {
		b.WriteString(t.Role)
		b.WriteString(": ")
		b.WriteString(t.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func (s *RAGService) appendHistory(sessionID string, turns ...Turn) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = append(s.sessions[sessionID], turns...)
	if len(s.sessions[sessionID]) > 20 {
		s.sessions[sessionID] = s.sessions[sessionID][len(s.sessions[sessionID])-20:]
	}
}

type qaItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func parseRerankIDs(raw string) []int64 {
	j := extractJSONArray(raw)
	if j == "" {
		return nil
	}

	var ids []int64
	if err := json.Unmarshal([]byte(j), &ids); err == nil && len(ids) > 0 {
		return ids
	}

	var objs []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(j), &objs); err == nil && len(objs) > 0 {
		for _, o := range objs {
			if o.ID > 0 {
				ids = append(ids, o.ID)
			}
		}
	}
	return ids
}

func parseQAPairs(raw string) []qaItem {
	j := extractJSONArray(raw)
	if j == "" {
		return nil
	}
	var out []qaItem
	_ = json.Unmarshal([]byte(j), &out)
	return out
}

func extractJSONArray(raw string) string {
	start := strings.Index(raw, "[")
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return ""
}
