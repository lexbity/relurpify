package capresult

import (
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
)

type ContentBlock interface {
	ContentType() string
}

type TextContentBlock struct {
	Text       string            `json:"text"`
	Provenance ContentProvenance `json:"provenance,omitempty"`
}

func (TextContentBlock) ContentType() string { return "text" }

type StructuredContentBlock struct {
	Data       any               `json:"data"`
	Provenance ContentProvenance `json:"provenance,omitempty"`
}

func (StructuredContentBlock) ContentType() string { return "structured" }

type ResourceLinkContentBlock struct {
	URI        string            `json:"uri"`
	Name       string            `json:"name,omitempty"`
	MIMEType   string            `json:"mime_type,omitempty"`
	Provenance ContentProvenance `json:"provenance,omitempty"`
}

func (ResourceLinkContentBlock) ContentType() string { return "resource-link" }

type EmbeddedResourceContentBlock struct {
	Resource   any               `json:"resource"`
	Provenance ContentProvenance `json:"provenance,omitempty"`
}

func (EmbeddedResourceContentBlock) ContentType() string { return "embedded-resource" }

type BinaryReferenceContentBlock struct {
	Ref        string            `json:"ref"`
	MIMEType   string            `json:"mime_type,omitempty"`
	Provenance ContentProvenance `json:"provenance,omitempty"`
}

func (BinaryReferenceContentBlock) ContentType() string { return "binary-reference" }

type ErrorContentBlock struct {
	Code       string            `json:"code,omitempty"`
	Message    string            `json:"message"`
	Provenance ContentProvenance `json:"provenance,omitempty"`
}

func (ErrorContentBlock) ContentType() string { return "error" }

type ContentProvenance struct {
	CapabilityID string                 `json:"capability_id,omitempty"`
	ProviderID   string                 `json:"provider_id,omitempty"`
	TrustClass   agentspec.TrustClass   `json:"trust_class,omitempty"`
	Disposition  ContentDisposition     `json:"disposition,omitempty"`
	Derivation   *ports.DerivationChain `json:"derivation,omitempty"`
}
