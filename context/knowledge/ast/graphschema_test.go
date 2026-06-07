package ast

import "testing"

func TestGraphSchemaConstantsCompileAndRemainDistinct(t *testing.T) {
	if NodeKindFunction == "" || NodeKindMethod == "" || NodeKindInterface == "" || NodeKindStruct == "" || NodeKindPackage == "" || NodeKindFile == "" {
		t.Fatal("expected graph node kind constants to be defined")
	}
	if EdgeKindViolatesContract == "" || EdgeKindDriftsFrom == "" {
		t.Fatal("expected semantic contract edge kinds to be defined")
	}
	if EdgeKindViolatesContract == EdgeKindCalls || EdgeKindDriftsFrom == EdgeKindCalls {
		t.Fatal("semantic contract edge kinds must be distinct from structural edge kinds")
	}
}
