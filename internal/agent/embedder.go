package agent

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

const embeddingDim = 512
const defaultChunkSize = 800

// EmbeddedDocument is a text chunk paired with its embedding vector.
type EmbeddedDocument struct {
	Filename  string
	Text      string
	Embedding []float32
}

// Embedder indexes documents and provides semantic search via local hash embeddings.
type Embedder struct {
	docs []EmbeddedDocument
}

// NewEmbedder creates a new Embedder.
func NewEmbedder() *Embedder {
	return &Embedder{}
}

// Index loads and embeds all .txt, .md, and .pdf files from dir.
// Returns without error if the directory is empty or does not exist.
func (e *Embedder) Index(dir string) error {
	chunks, err := loadChunks(dir)
	if err != nil {
		return fmt.Errorf("embedder: load chunks: %w", err)
	}

	if len(chunks) == 0 {
		log.Printf("embedder: no documents found in %q — search will return no results", dir)
		return nil
	}

	for _, c := range chunks {
		e.docs = append(e.docs, EmbeddedDocument{
			Filename:  c.filename,
			Text:      c.text,
			Embedding: embed(c.text),
		})
	}

	log.Printf("embedder: indexed %d chunks from %q", len(e.docs), dir)
	return nil
}

// Search returns the topK most relevant chunks for the given query.
func (e *Embedder) Search(query string, topK int) ([]EmbeddedDocument, error) {
	if len(e.docs) == 0 {
		return nil, nil
	}

	queryVec := embed(query)

	type scored struct {
		doc   EmbeddedDocument
		score float32
	}

	results := make([]scored, 0, len(e.docs))
	for _, doc := range e.docs {
		results = append(results, scored{doc: doc, score: cosineSimilarity(queryVec, doc.Embedding)})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	if topK > len(results) {
		topK = len(results)
	}

	out := make([]EmbeddedDocument, topK)
	for i := range out {
		out[i] = results[i].doc
	}
	return out, nil
}

// embed converts text into a fixed-size vector using feature hashing (no external model).
func embed(text string) []float32 {
	vec := make([]float32, embeddingDim)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		h.Write([]byte(word))
		vec[int(h.Sum32())%embeddingDim]++
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm = math.Sqrt(norm); norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}
	return vec
}

// ---- internal chunk helpers ----

type chunk struct {
	filename string
	text     string
}

func loadChunks(dir string) ([]chunk, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var chunks []chunk
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		var text string
		switch ext {
		case ".txt", ".md":
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			text = string(data)
		case ".pdf":
			var err error
			text, err = readPDF(filepath.Join(dir, name))
			if err != nil {
				return nil, fmt.Errorf("read pdf %q: %w", name, err)
			}
		default:
			continue
		}

		for _, c := range splitChunks(text, defaultChunkSize) {
			chunks = append(chunks, chunk{filename: name, text: c})
		}
	}

	return chunks, nil
}

func splitChunks(text string, maxLen int) []string {
	paragraphs := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")

	var chunks []string
	current := strings.Builder{}

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if current.Len() > 0 && current.Len()+len(p)+2 > maxLen {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(p)
	}

	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}

	return chunks
}

func readPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	plain, err := r.GetPlainText()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(plain); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}
