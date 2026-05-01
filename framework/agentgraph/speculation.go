package agentgraph

import (
	"context"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type speculativeJobEntry struct {
	job      *contextstream.Job
	storedAt time.Time
}

// SpeculationCache tracks background stream jobs keyed by node ID.
type SpeculationCache struct {
	mu   sync.Mutex
	ttl  time.Duration
	jobs map[string]speculativeJobEntry
}

// NewSpeculationCache creates a cache with the provided TTL.
func NewSpeculationCache(ttl time.Duration) *SpeculationCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &SpeculationCache{
		ttl:  ttl,
		jobs: make(map[string]speculativeJobEntry),
	}
}

// Store registers a speculative job for a node.
func (c *SpeculationCache) Store(nodeID string, job *contextstream.Job) {
	if c == nil || nodeID == "" || job == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked(time.Now().UTC())
	c.jobs[nodeID] = speculativeJobEntry{job: job, storedAt: time.Now().UTC()}
}

// Get returns a live speculative job if it exists and has not expired.
func (c *SpeculationCache) Get(nodeID string) (*contextstream.Job, bool) {
	if c == nil || nodeID == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cleanupTime := time.Now().UTC()
	c.cleanupLocked(cleanupTime)
	entry, ok := c.jobs[nodeID]
	if !ok || entry.job == nil {
		return nil, false
	}
	return entry.job, true
}

// Cleanup removes expired speculative jobs.
func (c *SpeculationCache) Cleanup() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked(time.Now().UTC())
}

func (c *SpeculationCache) cleanupLocked(now time.Time) {
	if c == nil || len(c.jobs) == 0 {
		return
	}
	for nodeID, entry := range c.jobs {
		if entry.job == nil {
			delete(c.jobs, nodeID)
			continue
		}
		if now.Sub(entry.storedAt) > c.ttl {
			delete(c.jobs, nodeID)
		}
	}
}

func (g *Graph) speculativeCompile(ctx context.Context, currentNodeID string, env *contextdata.Envelope, lookahead int) {
	if g == nil || env == nil || lookahead <= 0 {
		return
	}
	visited := map[string]struct{}{currentNodeID: struct{}{}}
	g.speculativeCompileFrom(ctx, currentNodeID, env, lookahead, visited)
}

func (g *Graph) speculativeCompileFrom(ctx context.Context, nodeID string, env *contextdata.Envelope, depth int, visited map[string]struct{}) {
	if g == nil || env == nil || depth <= 0 {
		return
	}
	g.mu.RLock()
	edges := append([]Edge(nil), g.edges[nodeID]...)
	g.mu.RUnlock()
	for _, edge := range edges {
		if _, seen := visited[edge.To]; seen {
			continue
		}
		visited[edge.To] = struct{}{}
		node, contract, ok := g.nodeAndContract(edge.To)
		if !ok {
			continue
		}
		if streamNode, ok := node.(*StreamTriggerNode); ok && contract.SpeculativeCompilationQuery != nil {
			g.launchSpeculativeJob(ctx, env, streamNode)
		}
		g.speculativeCompileFrom(ctx, edge.To, env, depth-1, visited)
	}
}

func (g *Graph) nodeAndContract(nodeID string) (Node, NodeContract, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[nodeID]
	if !ok {
		return nil, NodeContract{}, false
	}
	contract, ok := g.nodeContracts[nodeID]
	if !ok {
		contract = ResolveNodeContract(node)
	}
	return node, contract, true
}

func (g *Graph) launchSpeculativeJob(ctx context.Context, env *contextdata.Envelope, node *StreamTriggerNode) {
	if g == nil || node == nil || node.Trigger == nil || env == nil {
		return
	}
	if job, ok := node.speculativeJob(env); ok && job != nil {
		return
	}
	if g.speculation != nil {
		if job, ok := g.speculation.Get(node.ID()); ok && job != nil {
			node.storeJob(env, job)
			return
		}
	}
	job, err := node.Trigger.RequestBackground(ctx, node.request(env, true))
	if err != nil || job == nil {
		return
	}
	node.storeJob(env, job)
	if g.speculation != nil {
		g.speculation.Store(node.ID(), job)
	}
}
