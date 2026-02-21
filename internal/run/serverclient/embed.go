package serverclient

import "context"

// HTTPEmbedder implements model.Embedder via the server's /api/v1/embed endpoint.
type HTTPEmbedder struct {
	client *Client
	dims   int
}

// NewHTTPEmbedder creates an HTTPEmbedder. dims is the expected embedding dimension.
func NewHTTPEmbedder(client *Client, dims int) *HTTPEmbedder {
	return &HTTPEmbedder{client: client, dims: dims}
}

type embedRequest struct {
	Text string `json:"text"`
}

type embedResponse struct {
	Vector []float32 `json:"vector"`
	Model  string    `json:"model"`
	Dims   int       `json:"dims"`
}

// Embed sends text to the server and returns the embedding vector.
func (e *HTTPEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	var resp embedResponse
	if err := e.client.doJSON(ctx, "POST", "/api/v1/embed", embedRequest{Text: text}, &resp); err != nil {
		return nil, err
	}
	return resp.Vector, nil
}

// EmbedBatch embeds multiple texts by calling Embed sequentially.
func (e *HTTPEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		vec, err := e.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

// Dimensions returns the configured embedding dimensions.
func (e *HTTPEmbedder) Dimensions() int {
	return e.dims
}
