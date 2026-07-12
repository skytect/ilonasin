package provider

import (
	"bytes"
	"context"
	"testing"
	"time"
)

type recordingResponsesStreamSink struct {
	events [][]byte
}

func (s *recordingResponsesStreamSink) WriteEvent(_ context.Context, event []byte) error {
	s.events = append(s.events, append([]byte(nil), event...))
	return nil
}

func TestHandleCodexNativeResponsesEventRelaysTerminalFailures(t *testing.T) {
	tests := []struct {
		name       string
		block      []byte
		errorClass string
	}{
		{
			name:       "error",
			block:      []byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"code\":\"context_length_exceeded\",\"message\":\"input too long\",\"param\":\"input\"}}"),
			errorClass: "upstream_context_length_exceeded",
		},
		{
			name:       "response failed",
			block:      []byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"type\":\"invalid_request_error\",\"code\":\"context_length_exceeded\",\"message\":\"input too long\",\"param\":\"input\"}}}"),
			errorClass: "upstream_context_length_exceeded",
		},
		{
			name:       "response incomplete",
			block:      []byte("event: response.incomplete\ndata: {\"type\":\"response.incomplete\",\"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}"),
			errorClass: "upstream_response_incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.block[bytes.Index(tt.block, []byte("data: "))+len("data: "):]
			sink := &recordingResponsesStreamSink{}
			summary := ChatStreamSummary{}
			state := codexNativeResponsesState{requestedModel: "gpt-5.6-sol"}
			capture := upstreamStreamCapture{}

			err := (HTTPChatAdapter{}).handleCodexNativeResponsesEvent(
				context.Background(),
				bytes.Split(tt.block, []byte("\n")),
				[][]byte{data},
				sink,
				&summary,
				&state,
				time.Now(),
				&capture,
			)

			if err == nil {
				t.Fatal("expected terminal failure to remain classified as an error")
			}
			if summary.ErrorClass != tt.errorClass {
				t.Fatalf("expected error class %q, got %q", tt.errorClass, summary.ErrorClass)
			}
			if len(sink.events) != 1 {
				t.Fatalf("expected exactly one relayed event, got %d", len(sink.events))
			}
			if !bytes.Equal(sink.events[0], tt.block) {
				t.Fatalf("relayed event changed:\nwant: %q\n got: %q", tt.block, sink.events[0])
			}
			if !summary.Started || summary.ChunkCount != 1 {
				t.Fatalf("expected relayed event to update stream metrics, got started=%v chunks=%d", summary.Started, summary.ChunkCount)
			}
		})
	}
}
