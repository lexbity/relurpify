package errors_test

import (
	"errors"
	"testing"

	chainererrors "github.com/lexcodex/relurpify/agents/chainer/errors"
)

func TestLinkDecodeError(t *testing.T) {
	cause := errors.New("parse failed")
	err := &chainererrors.LinkDecodeError{
		LinkName:     "analyze",
		ResponseText: "invalid json",
		Cause:        cause,
		RetryCount:   2,
		MaxRetries:   3,
		LastAttempt:  "retry 2",
	}

	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected error chain")
	}
}

func TestLinkValidationError(t *testing.T) {
	cause := errors.New("schema mismatch")
	err := &chainererrors.LinkValidationError{
		LinkName:       "transform",
		OutputKey:      "result",
		ParsedOutput:   "something",
		ExpectedSchema: "json object with 'status' field",
		ValidationErr:  cause,
		RetryCount:     1,
		MaxRetries:     2,
	}

	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected error chain")
	}
}

func TestLinkApplyError(t *testing.T) {
	cause := errors.New("context locked")
	err := &chainererrors.LinkApplyError{
		LinkName:    "summarize",
		OutputKey:   "summary",
		OutputValue: "summary text",
		Cause:       cause,
	}

	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected error chain")
	}
}

func TestNilErrors(t *testing.T) {
	var decodeErr *chainererrors.LinkDecodeError
	if decodeErr.Error() != "" {
		t.Fatal("expected empty error for nil LinkDecodeError")
	}

	var validErr *chainererrors.LinkValidationError
	if validErr.Error() != "" {
		t.Fatal("expected empty error for nil LinkValidationError")
	}

	var applyErr *chainererrors.LinkApplyError
	if applyErr.Error() != "" {
		t.Fatal("expected empty error for nil LinkApplyError")
	}
}
