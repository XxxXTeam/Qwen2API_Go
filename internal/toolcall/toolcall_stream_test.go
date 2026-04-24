package toolcall

import "testing"

func TestProcessStreamChunkKeepsUTF8Boundary(t *testing.T) {
	state := NewStreamState()

	first := ProcessStreamChunk(state, "你好，世界")
	second := FinalizeStream(state)

	got := first.Content + second.Content
	want := "你好，世界"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}
