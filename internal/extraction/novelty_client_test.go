package extraction

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNoveltyClient_Novel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/novelty" {
			t.Errorf("expected /api/v1/novelty, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"novel":true,"score":0.95,"similar":[]}`))
	}))
	defer srv.Close()

	c := NewNoveltyClient(srv.URL, 0.7, slog.Default())
	res, err := c.Check(context.Background(), "some content")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Novel {
		t.Error("expected novel=true")
	}
	if res.Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", res.Score)
	}
}

func TestNoveltyClient_NotNovel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"novel":false,"score":0.3,"similar":[{"name":"existing-skill","score":0.92}]}`))
	}))
	defer srv.Close()

	c := NewNoveltyClient(srv.URL, 0.7, slog.Default())
	res, err := c.Check(context.Background(), "duplicate content")
	if err != nil {
		t.Fatal(err)
	}
	if res.Novel {
		t.Error("expected novel=false")
	}
	if len(res.Similar) != 1 {
		t.Fatalf("expected 1 similar, got %d", len(res.Similar))
	}
	if res.Similar[0].Name != "existing-skill" {
		t.Errorf("expected similar name 'existing-skill', got %q", res.Similar[0].Name)
	}
}

func TestNoveltyClient_ServerUnreachable(t *testing.T) {
	// Point at a port that nothing listens on.
	c := NewNoveltyClient("http://127.0.0.1:1", 0.7, slog.Default())
	c.httpClient.Timeout = 1 * time.Second

	res, err := c.Check(context.Background(), "content")
	if err != nil {
		t.Fatal("expected fail-open (no error), got", err)
	}
	if !res.Novel {
		t.Error("expected novel=true on unreachable server (fail-open)")
	}
}

func TestNoveltyClient_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewNoveltyClient(srv.URL, 0.7, slog.Default())
	_, err := c.Check(context.Background(), "content")
	if err == nil {
		t.Error("expected error on malformed response")
	}
}

func TestNoveltyClient_ServerTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.Write([]byte(`{"novel":true}`))
	}))
	defer srv.Close()

	c := NewNoveltyClient(srv.URL, 0.7, slog.Default())
	c.httpClient.Timeout = 100 * time.Millisecond

	res, err := c.Check(context.Background(), "content")
	if err != nil {
		t.Fatal("expected fail-open on timeout, got", err)
	}
	if !res.Novel {
		t.Error("expected novel=true on timeout (fail-open)")
	}
}
