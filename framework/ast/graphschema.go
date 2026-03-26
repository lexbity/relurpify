package ast

import (
	"encoding/json"

	"github.com/lexcodex/relurpify/framework/graphdb"
)

const (
	NodeKindFunction  graphdb.NodeKind = "function"
	NodeKindMethod    graphdb.NodeKind = "method"
	NodeKindInterface graphdb.NodeKind = "interface"
	NodeKindStruct    graphdb.NodeKind = "struct"
	NodeKindPackage   graphdb.NodeKind = "package"
	NodeKindFile      graphdb.NodeKind = "file"
	NodeKindImport    graphdb.NodeKind = "import"
	NodeKindType      graphdb.NodeKind = "type"
	NodeKindDocument  graphdb.NodeKind = "document"
	NodeKindSection   graphdb.NodeKind = "section"
)

const (
	EdgeKindCalls         graphdb.EdgeKind = "calls"
	EdgeKindCalledBy      graphdb.EdgeKind = "called_by"
	EdgeKindImports       graphdb.EdgeKind = "imports"
	EdgeKindImportedBy    graphdb.EdgeKind = "imported_by"
	EdgeKindImplements    graphdb.EdgeKind = "implements"
	EdgeKindImplementedBy graphdb.EdgeKind = "implemented_by"
	EdgeKindContains      graphdb.EdgeKind = "contains"
	EdgeKindContainedBy   graphdb.EdgeKind = "contained_by"
	EdgeKindDependsOn     graphdb.EdgeKind = "depends_on"
	EdgeKindDependencyOf  graphdb.EdgeKind = "dependency_of"
	EdgeKindReferences    graphdb.EdgeKind = "references"
	EdgeKindReferencedBy  graphdb.EdgeKind = "referenced_by"
	EdgeKindExtends       graphdb.EdgeKind = "extends"
	EdgeKindExtendedBy    graphdb.EdgeKind = "extended_by"
)

const (
	EdgeKindViolatesContract graphdb.EdgeKind = "violates_contract"
	EdgeKindDriftsFrom       graphdb.EdgeKind = "drifts_from"
)

func graphNodeRecord(node *Node, sourcePath string) (graphdb.NodeRecord, bool) {
	if node == nil {
		return graphdb.NodeRecord{}, false
	}
	kind, ok := graphNodeKind(node.Type)
	if !ok {
		return graphdb.NodeRecord{}, false
	}
	props, err := json.Marshal(map[string]any{
		"name":        node.Name,
		"signature":   node.Signature,
		"doc_string":  node.DocString,
		"file_id":     node.FileID,
		"language":    node.Language,
		"category":    node.Category,
		"start_line":  node.StartLine,
		"end_line":    node.EndLine,
		"is_exported": node.IsExported,
	})
	if err != nil {
		props = nil
	}
	return graphdb.NodeRecord{
		ID:        node.ID,
		Kind:      kind,
		SourceID:  sourcePath,
		Labels:    []string{string(node.Type), string(node.Category)},
		Props:     props,
		CreatedAt: node.CreatedAt.UnixNano(),
		UpdatedAt: node.UpdatedAt.UnixNano(),
	}, true
}

func graphNodeKind(nodeType NodeType) (graphdb.NodeKind, bool) {
	switch nodeType {
	case NodeTypeFunction:
		return NodeKindFunction, true
	case NodeTypeMethod:
		return NodeKindMethod, true
	case NodeTypeInterface:
		return NodeKindInterface, true
	case NodeTypeStruct:
		return NodeKindStruct, true
	case NodeTypePackage:
		return NodeKindPackage, true
	case NodeTypeImport:
		return NodeKindImport, true
	case NodeTypeType:
		return NodeKindType, true
	case NodeTypeDocument:
		return NodeKindDocument, true
	case NodeTypeSection, NodeTypeHeading:
		return NodeKindSection, true
	default:
		return graphdb.NodeKind(nodeType), true
	}
}

func graphEdgeKinds(edgeType EdgeType) (graphdb.EdgeKind, graphdb.EdgeKind, bool) {
	switch edgeType {
	case EdgeTypeCalls:
		return EdgeKindCalls, EdgeKindCalledBy, true
	case EdgeTypeImports:
		return EdgeKindImports, EdgeKindImportedBy, true
	case EdgeTypeImplements:
		return EdgeKindImplements, EdgeKindImplementedBy, true
	case EdgeTypeContains:
		return EdgeKindContains, EdgeKindContainedBy, true
	case EdgeTypeDependsOn:
		return EdgeKindDependsOn, EdgeKindDependencyOf, true
	case EdgeTypeReferences:
		return EdgeKindReferences, EdgeKindReferencedBy, true
	case EdgeTypeExtends:
		return EdgeKindExtends, EdgeKindExtendedBy, true
	default:
		return "", "", false
	}
}

func graphEdgeRecords(sourceID, targetID string, kind, inverseKind graphdb.EdgeKind, weight float32, props map[string]any) ([]graphdb.EdgeRecord, error) {
	raw, err := json.Marshal(props)
	if err != nil {
		return nil, err
	}
	edges := []graphdb.EdgeRecord{{
		SourceID: sourceID,
		TargetID: targetID,
		Kind:     kind,
		Weight:   weight,
		Props:    raw,
	}}
	if inverseKind != "" {
		edges = append(edges, graphdb.EdgeRecord{
			SourceID: targetID,
			TargetID: sourceID,
			Kind:     inverseKind,
			Weight:   weight,
			Props:    raw,
		})
	}
	return edges, nil
}
