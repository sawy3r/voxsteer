//go:build desktop && darwin

package desktop

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestEventChunksKeepTextIntact(t *testing.T) {
	for _, text := range []string{
		"",
		"short",
		strings.Repeat("a", maxEventUnits),
		strings.Repeat("abcdefghij", 9) + " ",
		"Café crème, naïve résumé, déjà vu. ",
		strings.Repeat("😀", 25),
		"Ship it 🚀 then celebrate 🎉🎉 with the 👩‍👩‍👧 ",
	} {
		chunks := eventChunks(text)
		var rebuilt strings.Builder
		for _, chunk := range chunks {
			if len(chunk) == 0 || len(chunk) > maxEventUnits {
				t.Fatalf("eventChunks(%q) has a chunk of %d units", text, len(chunk))
			}
			if isHighSurrogate(chunk[len(chunk)-1]) {
				t.Fatalf("eventChunks(%q) splits a surrogate pair", text)
			}
			rebuilt.WriteString(string(utf16.Decode(chunk)))
		}
		if rebuilt.String() != text {
			t.Fatalf("eventChunks(%q) rebuilds %q", text, rebuilt.String())
		}
	}
}

func TestEventChunksFillEachEvent(t *testing.T) {
	chunks := eventChunks(strings.Repeat("a", 45))
	if len(chunks) != 3 || len(chunks[0]) != 20 || len(chunks[1]) != 20 || len(chunks[2]) != 5 {
		t.Fatalf("chunk sizes = %v, want 20, 20, 5", chunkSizes(chunks))
	}
}

func TestEventChunksMoveAnEmojiThatWouldStraddleTwoEvents(t *testing.T) {
	chunks := eventChunks(strings.Repeat("a", maxEventUnits-1) + "😀b")
	if got := chunkSizes(chunks); len(got) != 2 || got[0] != maxEventUnits-1 || got[1] != 3 {
		t.Fatalf("chunk sizes = %v, want %d then 3", got, maxEventUnits-1)
	}
}

func chunkSizes(chunks [][]uint16) []int {
	sizes := make([]int, len(chunks))
	for i, chunk := range chunks {
		sizes[i] = len(chunk)
	}
	return sizes
}
