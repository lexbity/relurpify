package graphdb

import (
	"sort"
	"strings"
	"sync"
)

// LabelIndex maintains an inverted index from labels to node IDs.
type LabelIndex struct {
	mu      sync.RWMutex
	byLabel map[string]map[string]struct{}
}

func NewLabelIndex() *LabelIndex {
	return &LabelIndex{
		byLabel: make(map[string]map[string]struct{}),
	}
}

func (i *LabelIndex) Add(label, nodeID string) {
	if i == nil || label == "" || nodeID == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.byLabel == nil {
		i.byLabel = make(map[string]map[string]struct{})
	}
	ids := i.byLabel[label]
	if ids == nil {
		ids = make(map[string]struct{})
		i.byLabel[label] = ids
	}
	ids[nodeID] = struct{}{}
}

func (i *LabelIndex) Remove(label, nodeID string) {
	if i == nil || label == "" || nodeID == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	ids := i.byLabel[label]
	if len(ids) == 0 {
		return
	}
	delete(ids, nodeID)
	if len(ids) == 0 {
		delete(i.byLabel, label)
	}
}

func (i *LabelIndex) Lookup(label string) []string {
	if i == nil || label == "" {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	ids := i.byLabel[label]
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (i *LabelIndex) LookupPrefix(prefix string) []string {
	if i == nil || prefix == "" {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	seen := make(map[string]struct{})
	for label, ids := range i.byLabel {
		if !strings.HasPrefix(label, prefix) {
			continue
		}
		for id := range ids {
			seen[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (i *LabelIndex) Rebuild(nodes []NodeRecord) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.byLabel = make(map[string]map[string]struct{})
	for _, node := range nodes {
		if node.DeletedAt != 0 {
			continue
		}
		for _, label := range uniqueLabels(node.Labels) {
			if label == "" {
				continue
			}
			ids := i.byLabel[label]
			if ids == nil {
				ids = make(map[string]struct{})
				i.byLabel[label] = ids
			}
			ids[node.ID] = struct{}{}
		}
	}
}

func uniqueLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}
