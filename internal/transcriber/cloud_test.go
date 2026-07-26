package transcriber

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudTranscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "speech-model" {
			t.Errorf("model = %q", got)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		header := make([]byte, 4)
		_, _ = io.ReadFull(file, header)
		if string(header) != "RIFF" {
			t.Errorf("audio header = %q", header)
		}
		_, _ = io.WriteString(w, `{"text":" hello world "}`)
	}))
	defer server.Close()

	engine := &Cloud{Endpoint: server.URL, APIKey: "secret", Model: "speech-model"}
	got, err := engine.Transcribe(context.Background(), []float32{0, .25})
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world" {
		t.Fatalf("text = %q", got)
	}
}

func TestCloudErrorDoesNotLeakUnboundedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, strings.Repeat("x", 2<<20))
	}))
	defer server.Close()

	_, err := (&Cloud{Endpoint: server.URL}).Transcribe(context.Background(), []float32{0})
	if err == nil {
		t.Fatal("expected an error")
	}
}
