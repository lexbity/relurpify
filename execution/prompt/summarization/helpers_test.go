package summarization

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// chunkWithBody builds a knowledge chunk with a content_type field and raw body.
func chunkWithBody(id, contentType, body string) knowledge.KnowledgeChunk {
	return knowledge.KnowledgeChunk{
		ID:           knowledge.ChunkID(id),
		Version:      1,
		SourceOrigin: knowledge.SourceOriginFile,
		Body: knowledge.ChunkBody{
			Raw:    body,
			Fields: map[string]any{"content_type": contentType},
		},
	}
}

// fakeSummarizer is a scripted Summarizer used to drive the router.
type fakeSummarizer struct {
	kind   SummarizerKind
	can    bool
	result *SummarizationResult
	err    error
	gotReq SummarizationRequest
	calls  int
	canGot [][]knowledge.KnowledgeChunk
}

func (f *fakeSummarizer) Kind() SummarizerKind { return f.kind }

func (f *fakeSummarizer) CanSummarize(chunks []knowledge.KnowledgeChunk) bool {
	f.canGot = append(f.canGot, chunks)
	return f.can
}

func (f *fakeSummarizer) Summarize(_ context.Context, req SummarizationRequest) (*SummarizationResult, error) {
	f.calls++
	f.gotReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &SummarizationResult{Summary: "fake summary", UsedModel: false}, nil
}

// errFake is a stable sentinel: errors.Is must find it through the
// production wrap in Route/RouteWithKind.
var errFake = errors.New("scripted failure")

func fakeErr() error { return errFake }
