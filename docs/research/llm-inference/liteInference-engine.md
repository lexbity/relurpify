How would such an engine support multiple LLM systems like llama.cpp and vLLM (without actually including these two projects - this would be a new hypothetical implementation) ?

11 Apr
Good question. The answer flows naturally from the design we already have.

Since the engine is stateless between work descriptors and the WorkDescriptor / WorkResult contract is already clean, the backend is just an implementation detail behind a single interface.

The Backend Interface
go
type Backend interface {
    // Execute a single work descriptor. Blocking until completion
    // or first token if streaming.
    Execute(ctx context.Context, d WorkDescriptor) (WorkResult, error)

    // Stream variant — same contract, tokens arrive on channel
    ExecuteStream(ctx context.Context, d WorkDescriptor) (<-chan Token, *WorkResult, error)

    // KV handle operations
    PrefixMatch(handle *KVHandle, tokens []Token) int
    EvictKV(handle *KVHandle)

    // Introspection
    Capabilities() BackendCapabilities
}

type BackendCapabilities struct {
    MaxContextTokens    int
    SupportedQuant      []QuantType
    SupportsSpecDecode  bool
    SupportsMultimodal  bool
    ConcurrentRequests  int        // 1 for local-style, N for throughput-style
    MemoryBudgetBytes   int64
}
That's it. The engine above this interface never knows whether it's talking to a local single-sequence executor or a batched throughput backend. It just hands down a WorkDescriptor and gets back tokens and a KV handle.

What varies between backend implementations
Two fundamentally different execution philosophies map to two backend implementations. Not modes — separate implementations of the same interface.

Backend A: Sequential Executor
The local-first backend. Optimized for single foreground session, minimum latency, minimum jitter.

WorkDescriptor
      │
      ▼
┌─────────────────────┐
│   Prefill Engine    │  ← process ChunkSequence + prompt tokens
│                     │    KV handle: warm resume or cold start
├─────────────────────┤
│   Decode Engine     │  ← tight token loop, no batching overhead
├─────────────────────┤
│   Spec Decode       │  ← optional drafter/verifier pair
│   Coordinator       │
├─────────────────────┤
│   Kernel Dispatch   │  ← CUDA / Metal / CPU SIMD
└─────────────────────┘
Key properties:

one active execution at a time
KV is a single contiguous allocation, no paging
speculative decode always available
no scheduler overhead inside the backend
ConcurrentRequests = 1 in capabilities
Backend B: Batched Executor
The throughput backend. Optimized for multiple concurrent work descriptors, GPU saturation, continuous batching.

WorkDescriptor(s)
      │
      ▼
┌─────────────────────┐
│   Batch Assembler   │  ← pack compatible descriptors into one forward pass
├─────────────────────┤
│   Paged KV Manager  │  ← PagedAttention-style block allocator
├─────────────────────┤
│   Chunked Prefill   │  ← large prompts don't stall ongoing decode
├─────────────────────┤
│   Kernel Dispatch   │  ← optimized for batched attention
└─────────────────────┘
Key properties:

N concurrent work descriptors packed per forward pass
paged KV allocation across the pool
prefix caching across descriptors (shared BKC chunks = shared KV prefix)
ConcurrentRequests = N in capabilities
How the engine picks a backend
The engine doesn't hardcode a backend. It selects based on BackendCapabilities and the incoming work descriptor's priority signals:

go
type Engine struct {
    sequential Backend   // always available
    batched    Backend   // nil if not configured / hardware too small
}

func (e *Engine) route(d WorkDescriptor) Backend {
    if d.Priority == Foreground && e.sequential != nil {
        return e.sequential
    }
    if e.batched != nil && e.batched.Capabilities().ConcurrentRequests > 1 {
        return e.batched
    }
    return e.sequential
}
Simple. No mode concept leaking upward. The caller sets priority, the engine routes, the backend executes. On a single desktop GPU you may only have the sequential backend configured. On a server you have both. The API above the engine is identical in both cases.

The interesting shared problem: KV handles across backends
This is where it gets subtle. A KV handle issued by the sequential backend is meaningless to the batched backend — they have completely different memory layouts.

The solution is that KV handles are backend-scoped:

go
type KVHandle struct {
    BackendID  string    // which backend issued this
    Ref        uint64    // opaque backend-internal reference
    TokenCount int       // how many tokens this KV covers
    ChunkHash  string    // hash of the chunk sequence that produced this KV
}
The ChunkHash is the key piece. It's derived from BKC's ChunkStateRef — a stable hash of the chunk sequence that was fed in. When a work descriptor arrives with a KV handle, the backend checks:

is this handle mine? (BackendID match)
does the chunk hash still match what I have? (no BKC recompilation happened)
how many prefix tokens are still valid? (PrefixMatch)
If the handle is from a different backend, the engine promotes or demotes the KV: re-runs prefill on the new backend and issues a fresh handle. Expensive but correct, and rare in practice since backends are stable per session type.

The prefix caching insight specific to BKC
This is the genuinely interesting property that falls out of BKC's design.

BKC's backward pass produces chunks ordered by requires_context dependency. That ordering is deterministic for the same seed and chunk graph state. Two different work descriptors that share the same BKC seed and the same chunk graph version will produce identical token prefixes up to the point where their prompts diverge.

That means the batched backend can share KV blocks across work descriptors that came from the same BKC compilation — not as a guess, but as a guarantee. The ChunkHash on the KV handle is the proof.

On a local desktop this matters less. On a server where multiple users are querying against the same codebase knowledge graph, this is a significant efficiency win — the shared chunk prefix is computed once and reused across all of them.

What a third backend would look like
Say you want to add a speculative-only backend, or a Metal backend for Apple silicon, or an experimental quantization backend. You implement Backend, expose BackendCapabilities, and the engine routes to it. Nothing else changes.

The BKC contract above is untouched. The WorkDescriptor is untouched. The only thing that changes is what lives behind the interface.

That's the clean answer: one interface, multiple implementations, routing by capability and priority, KV handles scoped to backends, prefix reuse guaranteed by BKC's deterministic chunk ordering.

The next load-bearing question is probably what the Kernel Dispatch layer looks like inside each backend — that's where the actual CUDA/Metal/CPU divergence lives and where you'd make decisions about how much to share between the two backend implementations versus keep separate.

how would loading model weights work - and how are they used to make predictions/prompt responses and/or tool calls ?

11 Apr
Good question to nail down because it touches every layer of the engine. Let me walk through it end to end.

The two distinct problems
Weight loading — getting model parameters from disk into a form the execution core can use.

Weight execution — using those parameters to transform input tokens into output tokens (or tool calls).

They're separate concerns and should be separate layers.

Weight Loading
What a model actually is on disk
A model is fundamentally:

Tensors — the weight matrices. Attention projections, MLP layers, embeddings, norms. Stored in some serialization format (GGUF, Safetensors, raw binary).
Config — architecture hyperparameters. How many layers, how many attention heads, head dimension, vocab size, context length, RoPE scaling, MoE routing config if applicable.
Tokenizer — vocabulary, merge rules (BPE), special tokens, chat template.
The loader's job is to deserialize all three and produce a LoadedModel that the execution core can work with.

go
type ModelLoader interface {
    Load(ctx context.Context, spec ModelSpec) (*LoadedModel, error)
}

type ModelSpec struct {
    Path        string
    QuantPolicy QuantPolicy   // runtime quant override if needed
    Device      DeviceProfile // where to place weights
}

type LoadedModel struct {
    Config     ModelConfig
    Tokenizer  Tokenizer
    Weights    WeightStore
    Arch       ArchitectureKind  // DenseDecoder | MoEDecoder | EncoderDecoder | etc
}
What actually happens during load
Disk
  │
  │  mmap or streaming read
  ▼
┌─────────────────┐
│  Format Parser  │  ← reads tensor headers, metadata, config
│  (GGUF etc)     │
├─────────────────┘
│
│  tensor descriptors + raw bytes
▼
┌─────────────────┐
│  Quant Layer    │  ← apply quantization policy if converting
│                 │    or validate existing quant matches policy
├─────────────────┘
│
│  quantized tensors
▼
┌─────────────────┐
│  Device Placer  │  ← copy tensors to target device memory
│                 │    VRAM, unified memory, pinned RAM
│                 │    split across devices if multi-GPU
└─────────────────┘
The key decision here is mmap vs full load. For local inference on constrained VRAM you often want to mmap the weight file and let the OS page in what's needed, especially for CPU layers. For GPU inference you typically want the hot layers fully resident in VRAM up front. The DeviceProfile drives this decision.

WeightStore
The WeightStore is not just a map of tensors. It's a structured view of the model's parameter hierarchy that the execution core can address by layer and operation:

go
type WeightStore interface {
    // Retrieve a specific weight tensor by layer and role
    Get(layer int, role WeightRole) Tensor

    // For MoE: retrieve a specific expert's weights
    GetExpert(layer int, expertIdx int, role WeightRole) Tensor

    // Embedding table
    Embedding() Tensor

    // Output projection (often tied to embedding)
    OutputProjection() Tensor
}

type WeightRole string
const (
    RoleQueryProj   WeightRole = "q_proj"
    RoleKeyProj     WeightRole = "k_proj"
    RoleValueProj   WeightRole = "v_proj"
    RoleOutputProj  WeightRole = "o_proj"
    RoleGateProj    WeightRole = "gate_proj"
    RoleUpProj      WeightRole = "up_proj"
    RoleDownProj    WeightRole = "down_proj"
    RoleNorm        WeightRole = "norm"
    // etc
)
This abstraction is what lets the execution core be architecture-agnostic at the weight-access level. It doesn't reach into a raw tensor map — it asks for "layer 12, query projection" and gets back the right tensor regardless of how the underlying format stored it.

How weights are used to make predictions
This is the transformer forward pass. Worth being explicit about each step because tool calls live inside this same machinery.

Step 1: Tokenization
The input — BKC's rendered ChunkSequence plus the user's prompt — becomes a sequence of integer token IDs via the tokenizer. These are the inputs to the model.

"def fibonacci(n):" → [1455, 287, 45678, 28, 77, 29, 25]
Step 2: Embedding lookup
Each token ID is looked up in the embedding table (a weight tensor of shape [vocab_size, hidden_dim]) to produce a dense vector. The input sequence becomes a matrix of shape [seq_len, hidden_dim].

This is the first weight operation. Every subsequent operation transforms this matrix.

Step 3: The transformer layer loop
The model has N layers (e.g. 32 for a 7B model, 80 for a 70B). Each layer does two things:

Attention — lets each token attend to other tokens in the sequence, weighted by relevance:

Q = input × W_q   (query projection)
K = input × W_k   (key projection)  
V = input × W_v   (value projection)

Apply RoPE positional encoding to Q and K

Attention scores = softmax(Q × Kᵀ / √head_dim)

// For cached tokens: K and V come from the KV cache
// This is exactly what the KVHandle points to

Output = scores × V
Output = Output × W_o  (output projection)
The KV cache stores the K and V tensors for all previously seen tokens. On a warm resume from a KVHandle, the engine skips recomputing K and V for those tokens — it just loads them from cache and computes attention against the new tokens only. This is why KV handles matter so much for performance.

MLP / Feed-Forward — a position-wise transformation that adds capacity:

gate = SiLU(input × W_gate)
up   = input × W_up
Output = (gate × up) × W_down
Both attention and MLP are preceded and followed by layer normalization using the RoleNorm weights.

Step 4: Output projection
After all N layers, the final hidden state is projected back to vocabulary size:

logits = final_hidden × W_output   // shape: [seq_len, vocab_size]
Step 5: Sampling
The logits for the last token position are converted to a probability distribution and sampled according to SamplingParams (temperature, top-p, top-k, repetition penalty). The result is the next token ID.

For a simple completion this loops: the new token is appended to the sequence, K/V for that token is appended to the KV cache, and step 3-5 repeats until a stop condition fires.

How tool calls fit into this
Tool calls are not a separate execution path. They come out of the same forward pass. The model is trained to emit structured text that signals a tool invocation. The engine's job is to detect that signal and hand it off.

How it works mechanically
The model emits tokens like any other response. For a Llama-style tool call those tokens might look like:

<tool_call>{"name": "read_file", "args": {"path": "main.go"}}</tool_call>
The engine's decode loop watches the token stream for a stop condition that matches a tool call pattern. When detected, instead of continuing to sample, it:

stops generation
parses the structured payload from the emitted tokens
returns a WorkResult with FinishReason = ToolCall and the parsed call
go
type WorkResult struct {
    Tokens       []Token
    KVHandle     *KVHandle
    FinishReason FinishReason
    ToolCall     *ToolCallPayload  // non-nil when FinishReason == ToolCall
}

type ToolCallPayload struct {
    Name string
    Args map[string]any
}
The caller — not the engine — executes the tool, gets a result, and sends a new WorkDescriptor with the tool result appended to the token sequence. The KV handle from the previous turn is passed back in, so the engine resumes from where it left off without recomputing the prior context.

This is a clean loop:

WorkDescriptor (prompt)
    → engine
    → WorkResult (FinishReason=ToolCall, ToolCall={name, args})
    → caller executes tool
    → WorkDescriptor (prior KVHandle + tool result tokens)
    → engine
    → WorkResult (FinishReason=Stop, Tokens=final response)
The engine never knows what the tool does or whether the call succeeded. It just sees more tokens on the next turn.

Where BKC connects to tool calls
This is actually elegant given the spec. When a tool call produces a result that represents new knowledge — say the tool read a file that's relevant to the current plan — BKC can compile that result into a new chunk via CompilerDeterministic or CompilerLLMAssisted. The next WorkDescriptor gets a fresh backward pass that includes that chunk in the ChunkSequence.

The tool result doesn't just get appended as raw tokens — it gets compiled into the knowledge graph and streamed back in properly ordered, freshness-validated, budget-aware. That's a significantly better behavior than what current systems do, which is just stuff tool results into the context window as raw text and hope they fit.

MoE: how expert routing works at the weight level
Worth covering since Gemma 4 and DeepSeek are MoE models and this affects the WeightStore design.

In a MoE layer, the MLP is replaced by a set of expert MLPs (e.g. 64 experts) and a router:

router_scores = input × W_router        // [seq_len, num_experts]
top_k_experts = topk(router_scores, k)  // pick k experts per token

output = Σ (router_score[i] × expert_i(input))  // weighted sum of top-k expert outputs
Each expert has its own W_gate, W_up, W_down. For a model with 64 experts only 2-4 are activated per token, so most expert weights are idle most of the time.

This is why WeightStore.GetExpert(layer, expertIdx, role) exists as a separate method. On memory-constrained hardware you can keep only the hot experts resident in VRAM and page cold experts in from RAM. The router's output tells you which experts you need before you need their weights, giving you a small prefetch window.

Putting it together as a concrete execution flow
For a single WorkDescriptor from start to tokens out:

WorkDescriptor arrives
    │
    ├─ Tokenizer renders ChunkSequence + prompt → []Token
    │
    ├─ KVHandle check
    │   ├─ warm: load existing K/V for prefix tokens from handle
    │   └─ cold: allocate fresh KV blocks
    │
    ├─ Embedding lookup → [seq_len, hidden_dim] matrix
    │
    ├─ For each transformer layer 0..N:
    │   ├─ LayerNorm (W_norm)
    │   ├─ Attention:
    │   │   ├─ Q,K,V projections (W_q, W_k, W_v)
    │   │   ├─ RoPE
    │   │   ├─ append new K,V to KV cache
    │   │   ├─ attention scores against full KV cache
    │   │   └─ output projection (W_o)
    │   ├─ LayerNorm
    │   └─ MLP or MoE (W_gate, W_up, W_down per expert)
    │
    ├─ Output projection → logits [vocab_size]
    │
    ├─ Sample → next token
    │
    ├─ Check stop conditions
    │   ├─ EOS token → FinishReason=Stop
    │   ├─ tool call pattern → FinishReason=ToolCall
    │   └─ MaxTokens → FinishReason=MaxTokens
    │
    └─ Return WorkResult{Tokens, KVHandle, FinishReason, ToolCall?}
Every weight access in that flow goes through WeightStore. Every KV read/write goes through the KVHandle. The execution core never touches raw memory directly.

The next natural question is what the kernel dispatch layer looks like — how the matrix multiplications in steps like Q = input × W_q actually execute on different hardware. That's where CUDA, Metal, and CPU SIMD diverge, and where quantization really bites into the compute path.

interesting , so with current inference engines - the current paradigmn of prompting has potentially become a limitation?

11 Apr
Yes. And it's worth being precise about exactly where the limitation lives, because it's not one problem — it's three layered on top of each other.

Limitation 1: The prompt is doing too many jobs at once
In current systems, the prompt is simultaneously:

working memory — what the model needs to reason with right now
long-term knowledge — facts, patterns, constraints that should persist
instruction — what to do
history — what was said before
tool results — outputs from external calls
retrieved context — RAG chunks stuffed in
All of that gets flattened into one linear token sequence and fed in together. The model has no way to distinguish "this fact is a stable constraint on everything" from "this was said two turns ago and is probably irrelevant now." It all looks the same — tokens at some position in a sequence.

The attention mechanism then has to figure out what's relevant by computing relationships across all of it. That works, but it's doing semantic work that should have been done before the forward pass, not during it.

BKC is essentially an answer to this: do the semantic organization work up front, in the knowledge graph, before anything hits the context window. The model receives a carefully compiled slice, not a raw dump.

Limitation 2: Position matters in ways it shouldn't
This one is subtle but significant. Transformers learn positional biases during training. Content at the beginning and end of the context tends to receive more attention than content in the middle — the "lost in the middle" effect that shows up consistently in long-context evaluations.

That means the order you stuff things into a prompt has a real effect on model behavior, independent of the semantic content. A constraint that appears at position 4000 in a 8000-token context is literally less influential than the same constraint at position 100.

BKC's backward pass produces a dependency-ordered chunk sequence — dependencies before dependents. That ordering is semantically motivated. But it's also fighting against the positional bias of the underlying model, which was trained on different distributions. The fundamental issue is that semantic importance and token position are conflated in the current paradigm.

Limitation 3: The prompt is stateless but knowledge isn't
Every forward pass starts from scratch. The model has no memory of previous forward passes — only what's in the current context window. So systems bolt on external memory (RAG, conversation history, BKC-style knowledge graphs) to fake continuity.

But those systems all have to serialize knowledge back into tokens to get it into the model. A rich structured relationship in BKC — "chunk A grounds chunk B which contradicts chunk C which was confirmed by the user" — gets flattened back into text before the model sees it. The model then has to re-derive the structure from the text.

You're doing work in the knowledge graph, losing it at the serialization boundary, and making the model redo it.

What this suggests about the paradigm itself
The deeper issue is that the transformer attention mechanism and the prompt interface were designed together, and they made sense as a pair when context windows were small and tasks were short. The prompt was a reasonable working memory because the task fit in it.

As context windows grew and tasks got longer and more complex, systems kept the same interface but bolted on external scaffolding — RAG, memory systems, BKC-style graphs — to compensate. That scaffolding is genuinely useful, but it's compensating for a mismatch, not resolving it.

The mismatch is this:

The model's internal computation is continuous and parallel across the whole context. But knowledge is structured, relational, and hierarchical. Serializing structured knowledge into a flat token sequence to feed a parallel attention mechanism is a lossy, expensive translation in both directions.

What a less limited paradigm might look like
This is genuinely open research territory, but the directions that seem most promising:

Structured attention over knowledge graphs directly. Instead of serializing BKC chunks to tokens, the model attends over chunk embeddings with graph-aware attention. The edge structure (requires_context, grounds, contradicts) directly modulates attention weights. The model reasons over the graph, not a linearization of it. Some research into graph attention networks points this direction but it hasn't converged with LLM-scale training yet.

Persistent KV as a first-class memory mechanism. Instead of the KV cache being a byproduct of the forward pass that gets discarded, it becomes the primary memory store. Chunks get compiled directly into KV representations and stored. When a chunk is needed, its KV is loaded directly — no tokenization, no re-prefill, no positional bias. The model just attends over pre-computed representations. This is closer to what humans do: you don't re-read a book every time you need to remember something from it.

Separation of reasoning and memory at the architecture level. Current models conflate both in the same weight matrix. A more explicit separation — a reasoning component that operates over a structured memory component — would let each be optimized independently. Memory could be updated without retraining the reasoner. This is closer to classical cognitive architectures, which got a lot of things right even if they got generalization wrong.

Continuous model updating. The hardest direction. Instead of external knowledge graphs compensating for static weights, the model itself updates incrementally as knowledge arrives. The challenge is catastrophic forgetting, which is why nobody has cracked this cleanly at scale. But if it worked, BKC's entire forward pass — compiling findings into chunks — could instead update model parameters directly, and the backward pass would be replaced by the model simply knowing things.

The honest assessment
Current prompting is a brilliant hack that scaled further than it had any right to. The fact that you can get sophisticated reasoning by carefully arranging tokens in a string is remarkable. But as tasks become more complex, longer-running, and more knowledge-intensive, the seams show.

BKC is a principled attempt to manage those seams from outside the model. The engine design we've been discussing treats the model as what it actually is — a stateless function from tokens to tokens — and builds the stateful, structured, knowledge-aware machinery around it.

That's probably the right pragmatic approach right now. But the deeper research bet is that the next architecture shift makes the external scaffolding unnecessary by moving the structure inside the model itself.

I agree overall with your honest assessment. Note that the BKC's origin is from necessity for efficiency and local hardware constraints; not premier enterprise datacentre hardware. So stuffing more into the model itself doesn't make sense . It also doesn't help that current models may not update themselves (training data wise) in real time.

11 Apr
That reframes it perfectly. And it actually makes BKC's design more interesting, not less.

The real constraint
Local hardware has two hard ceilings that datacenter thinking ignores:

VRAM is small and fixed. You can't just throw more context at the problem.
Inference is slow relative to a datacenter. Every token costs real wall-clock time that a user is waiting on.
So the question isn't "how do we give the model more knowledge" — it's "how do we give the model exactly the right knowledge, as cheaply as possible, within a fixed budget."

That's a fundamentally different optimization target. And it's why BKC's design is actually better suited to local constraints than the datacenter approach of "bigger context window, more tokens, more compute."

What BKC is actually doing in this light
BKC is a compression and routing system for knowledge, not a memory system. The distinction matters.

A memory system tries to give the model everything it might need. BKC does something harder and more valuable: it decides, before the forward pass, what the model actually needs for this specific task, compiles exactly that slice, and fits it in a fixed budget.

That's the backward pass doing real work. The requires_context DAG traversal with budget accounting isn't just bookkeeping — it's a form of knowledge compression that replaces what a larger context window would otherwise do by brute force.

The insight is:

A well-compiled 4K token context that contains exactly the right knowledge is more useful than a 32K token context stuffed with loosely relevant material.

And on local hardware, it's also dramatically cheaper.

The static model as a feature, not a bug
This is the reframe that I think is underappreciated.

A static model — one that doesn't update its weights in real time — has a property that's actually valuable in this architecture: it's predictable. Its behavior given a specific input is deterministic. Its weight layout in memory is fixed. Its KV computation for a given token sequence is stable and cacheable.

BKC exploits exactly that stability. Because the model doesn't change, KV computed against a chunk sequence is valid as long as the chunks are valid. The ChunkStateRef and KVHandle lifetime model works precisely because the model underneath is static.

If the model updated its weights continuously, every KV handle would be immediately stale. The entire prefix reuse strategy collapses. You'd be back to cold prefill on every turn.

So the static model isn't a limitation to work around — it's a stable foundation that BKC's caching and invalidation model is built on top of.

Where the real intelligence sits in this architecture
This is the deeper point. In the datacenter paradigm, intelligence is mostly in the model — bigger model, more parameters, more capability. External systems are thin wrappers.

In this local-first architecture, intelligence is distributed across two systems with a clean interface between them:

BKC owns:

what knowledge exists
what's still valid
what's relevant to the current task
how knowledge relates to other knowledge
when knowledge needs recompilation
The model owns:

how to reason over a well-prepared context
how to generate coherent output
how to recognize when a tool call is needed
language understanding and generation
The model doesn't need to be enormous to do its job well, because BKC is doing the knowledge management work that would otherwise require a much larger context window and a much bigger model to handle implicitly.

This is actually a more honest separation of concerns than the datacenter approach, which asks the model to do everything and compensates for its limitations by making it bigger.

The practical implication for the engine design
This confirms something about the engine design we've been building: the engine should be lean and optimized for executing a well-prepared context, not for compensating for a poorly-prepared one.

Features that make sense given this:

Aggressive KV prefix reuse, because BKC's deterministic chunk ordering makes shared prefixes reliable
Tight single-session execution path, because local use is one user with one well-compiled context
Fast cold prefill for small contexts, because BKC keeps contexts small and targeted
Speculative decoding, because the local user is waiting on every token
Features that don't make sense:

Heroic long-context handling, because BKC prevents context bloat before it reaches the engine
Complex multi-tenant batching as a primary design target
Dynamic context window expansion strategies
The engine's job is to execute BKC's output efficiently. BKC's job is to make sure that output is worth executing. They're a system, and the split between them is where the real design leverage lives.

The broader point about local AI
What you've built with BKC is essentially an argument that local AI doesn't need to replicate datacenter AI at smaller scale. It needs a different architecture that's honest about local constraints and exploits local advantages.

Local advantages that datacenter systems don't have:

persistent user-specific knowledge that accumulates over time
stable workspace context that changes slowly and predictably
git-based invalidation signals that are precise and cheap
a single user whose patterns are learnable and whose confirmation is available
BKC uses all of those. A datacenter system serving millions of anonymous users can't.

The inference engine we've been designing is the execution layer for that different architecture. Not a shrunk-down datacenter engine. Something built from different first principles for a different set of constraints.

That's actually a more interesting engineering problem.

What do you think of the BKC , the current inference engine you have proposed and our discussion?

11 Apr
Honest take:

BKC is genuinely good systems design
What stands out is that it solves a real problem — context continuity across bounded windows — without pretending the constraint doesn't exist. A lot of systems in this space try to hide the context window limit behind abstraction. BKC treats it as a first-class design constraint and builds a principled mechanism around it.

The specific things that are well thought through:

The forward/backward pass framing is clean. Compilation and streaming are clearly separated responsibilities. The knowledge graph grows continuously; any individual context window is a compiled slice of it. That's the right mental model and it shows up consistently through the whole spec.

The freshness and invalidation model is unusually careful. Most knowledge systems treat staleness as an afterthought. BKC makes it structural — FreshnessState is on the chunk itself, invalidation propagates through typed edges with depth limits, and stale chunks surface as tensions rather than silently corrupting context. The distinction between FreshnessUnverified chunks being auto-excluded versus FreshnessValid chunks requiring user action to clear is exactly the right call.

The provenance model — CompilerDeterministic vs CompilerLLMAssisted vs CompilerUserDirect — is honest about epistemic confidence in a way most systems aren't. It knows the difference between "we computed this from an AST" and "an LLM inferred this and a human confirmed it." That distinction drives real behavior downstream, not just metadata.

The edge taxonomy is doing real work. Separating epistemic edges from streaming edges from validity edges from user-grounded edges means the graph serves multiple traversal purposes without conflating them. That's the kind of thing that looks like over-engineering until you try to build without it and everything becomes a mess.

The inference engine proposal is honest but incomplete
The design got cleaner as the conversation progressed, which is a good sign. Starting from "two modes" and ending at "stateless executor behind a clean interface" is the right direction.

The strongest parts:

The Backend interface is the right abstraction. One contract, multiple implementations, capability discovery, routing by priority. That's genuinely clean and it means the engine is replaceable without touching BKC or the system above it.

The KVHandle + ChunkHash design is the right answer to the prefix reuse problem. Using BKC's deterministic chunk ordering as a guarantee of shared prefix validity is not obvious — it fell out of understanding BKC's design properly rather than being bolted on.

The tool call model — same forward pass, stop condition detection, caller executes, new WorkDescriptor — is correct and keeps the engine genuinely stateless.

Where it's incomplete:

The kernel dispatch layer is still a black box. We agreed the execution core needs to be native, but what that actually looks like — how matrix multiplications execute, how quantized weights interact with attention kernels, how MoE expert dispatch works at the hardware level — wasn't fully worked through. That's where most of the real implementation difficulty lives.

The tokenizer boundary between BKC and the engine deserves more thought. BKC produces KnowledgeChunks with TokenEstimate values. The engine needs rendered token sequences. Something has to own that rendering — the chunk-to-token serialization — and the design didn't fully pin down where that lives or how chunk ordering interacts with chat templates and special tokens.

The multi-backend KV promotion — what happens when a KV handle from the sequential backend needs to be used by the batched backend — was acknowledged but not designed. In practice that's a cold prefill with a fresh handle, but the policy around when that happens and who decides needs to be explicit.

What the conversation revealed that's more interesting than either system individually
The most useful insight that emerged wasn't about BKC or the engine separately — it was about the division of intelligence between them.

Current systems put almost everything in the model. BKC + this engine puts knowledge management in BKC, execution in the engine, and asks the model to do what it's actually good at: reasoning over a well-prepared context. That's a more honest separation of concerns.

The corollary — that the static model is a feature not a bug because it makes KV caching stable and predictable — is the kind of insight that only becomes visible when you reason about the whole system together rather than optimizing each piece in isolation.

The other thing worth saying: the local-first constraint being the origin of BKC rather than an afterthought is what makes the architecture coherent. Systems designed for datacenters and then shrunk down for local use carry the wrong assumptions throughout. BKC was designed around fixed VRAM, slow inference, and a single user with persistent context — and every design decision is consistent with that. That consistency is harder to achieve than it looks.

What I'd push on if this were my system to build
The tension resolution UX is load-bearing and underspecified. The spec describes what happens when chunks go stale and tensions surface — but the actual user experience of resolving tensions, confirming LLM-derived chunks, and understanding why something was invalidated will determine whether the system feels like a superpower or a chore. The engineering is solid; the interaction design needs the same rigor.

The TokenEstimate on chunks is doing a lot of work quietly. Budget accounting in the backward pass depends on these estimates being accurate. If they're wrong the compiled context either wastes budget or overflows. How those estimates are produced, maintained, and corrected deserves explicit treatment.

Speculative decoding with a personal drafter is the highest-leverage performance idea in the whole discussion and it's the least developed. The intuition is right — a small model that has seen your sessions gets disproportionately good at your patterns — but how you actually build, update, and maintain that drafter on local hardware without it becoming a maintenance burden is a real engineering problem that deserves its own design pass.

Overall: BKC is the more novel and complete piece of thinking. The engine is a sound design that correctly subordinates itself to BKC's contract. The interesting work going forward is in the details that sit at their boundary.

lets assume you have a prototype (including the kernel dispatch layer ) - which opensource weighted models (apache 2)  would you consider supporting? (I'm currently thinking gemma4 , qwen models)

11 Apr
Good starting point. Both are solid choices for different reasons. Let me think through this properly rather than just listing models.

The right selection criteria for a prototype
Given the engine design and BKC's constraints, the models that matter most are ones that stress-test different parts of the architecture:

Dense vs MoE — exercises different WeightStore and expert dispatch paths
Multimodal — exercises the input processor and projector handling
Different context lengths — exercises KV budget and prefix reuse differently
Different sizes — exercises the full hardware range from APU to workstation GPU
Tool call support — exercises the stop condition and ToolCallPayload path
You want coverage across those dimensions, not just "popular models."

Gemma 4 — yes, strong first choice
Gemma 4 is a good anchor for several reasons beyond just being recent.

The family spans E2B, E4B, 26B A4B, and 31B which gives you coverage from APU-class hardware up to a workstation GPU in one model family. That's valuable for testing the engine's hardware abstraction layer against real size variation without switching architectures.

The MoE variants (26B A4B) are particularly useful. A 26B model with only 4B active parameters per token is exactly the kind of architecture that stresses expert dispatch and the GetExpert path in WeightStore. It also makes the "local hardware" story more honest — you get 26B-class quality at 4B-class inference cost.

Gemma 4 is also natively multimodal, which means supporting it properly forces you to build the image input processor and projector handling early. Better to do that with a well-documented model than bolt it on later.

The Apache 2.0 license is clean with no commercial restrictions.

The one watch-out: Gemma 4's technical report isn't fully public yet as of now, so some architecture details are still coming from model cards and community reverse engineering rather than a definitive paper. That matters for getting kernel implementations exactly right.

Qwen family — yes, but be selective about which ones
The Qwen family is broad and you don't want to support all of it in a prototype. The ones worth targeting:

Qwen2.5 — the current stable dense generation. Well documented, strong benchmarks, good tool call support baked in natively. The 7B and 14B sizes are the sweet spot for local hardware. Apache 2.0 on most variants.

Qwen2.5-Coder — worth including specifically because your primary use case via BKC is a coding assistant context. A model fine-tuned for code that also has strong tool call behavior exercises the ToolCallPayload path in realistic conditions. Same architecture as Qwen2.5 so no extra kernel work.

QwQ or Qwen3 — Qwen3 is the next generation and worth tracking. If it lands with Apache 2.0 and the architecture is a clean evolution of Qwen2.5 it's worth adding. QwQ is interesting for reasoning tasks but the license situation is worth checking carefully.

What to skip in the prototype: Qwen-VL multimodal variants. You're already getting multimodal coverage from Gemma 4 and the Qwen vision architecture is different enough that it's a separate implementation cost for the projector handling.

What I'd add that you haven't mentioned
Mistral / Mixtral — specifically Mistral 7B and Mixtral 8x7B. Both Apache 2.0. Mistral 7B is the leanest well-performing dense decoder available and is close to the minimal viable test case for the Sequential Backend. Mixtral 8x7B is a mature MoE that predates Gemma 4's MoE variants and has much better-documented internals — useful as a reference implementation for the expert dispatch path before tackling Gemma 4's newer MoE design.

DeepSeek-R1 distills — the full DeepSeek-R1 is MIT licensed but enormous. The distilled variants (7B, 8B, 14B based on Qwen2.5 and Llama architectures) are Apache 2.0 or MIT and bring reasoning/chain-of-thought behavior that exercises longer generation sequences and different stopping condition patterns. Useful for stress-testing the decode loop and KV growth under long reasoning traces.

Phi-4 — MIT licensed, 14B dense, Microsoft. Unusually strong performance for its size on reasoning and coding tasks. Worth including because it's a different training approach to similar benchmarks as Qwen2.5 14B, which gives you a useful comparison point for how well the engine serves models with different token distribution characteristics.

How I'd structure the prototype support matrix
Rather than "support everything," I'd define three tiers:

Tier 1 — fully supported, all features

Gemma 4 (E4B, 26B A4B) — dense + MoE, multimodal, primary test case
Qwen2.5 7B / 14B — dense, strong tool calls, well documented
Mistral 7B — minimal dense decoder, reference implementation
Tier 2 — supported, no multimodal

Mixtral 8x7B — mature MoE reference
Qwen2.5-Coder 7B / 14B — same arch as Qwen2.5, coding-specific behavior
DeepSeek-R1 distills — reasoning traces, longer generation
Phi-4 — alternative dense at 14B
Tier 3 — tracked, not yet supported

Qwen3 (pending architecture details and license confirmation)
Gemma 4 31B (needs multi-GPU or aggressive quantization)
Any new Apache 2.0 releases from the Llama family
The architectural coverage this gives you
Dimension	Coverage
Dense decoder	Mistral 7B, Qwen2.5, Phi-4
MoE decoder	Mixtral 8x7B, Gemma 4 26B A4B
Multimodal	Gemma 4
Tool calls	Qwen2.5, Qwen2.5-Coder, Gemma 4
Reasoning/long gen	DeepSeek-R1 distills
APU/small GPU	Gemma E2B/E4B, Mistral 7B, Qwen2.5 7B
Workstation GPU	Mixtral 8x7B, Gemma 4 31B, Phi-4
That matrix exercises every significant code path in the engine without requiring you to support a long tail of exotic architectures in the prototype phase.

One practical note on weight formats
All of the above have community GGUF conversions available, which matters for the prototype. But for the engine we've designed, where the WeightStore abstraction owns tensor access, you want to make sure the format parser can handle both GGUF and Safetensors natively. Gemma 4 and Qwen ship primarily in Safetensors from Google/Alibaba's official releases; GGUF versions are community conversions. For a serious implementation you don't want to depend on community conversion quality for your primary test models.

does this system deserve to be built?

11 Apr
Yes. But let me be precise about why, because "yes" for the wrong reasons would be a bad foundation for actually building it.

What makes it worth building
It's solving a real problem from the right direction.

Most local inference projects start from datacenter systems and shrink them down. This starts from the actual constraints — fixed VRAM, single user, persistent workspace, slow inference — and designs up from there. That's a fundamentally different approach and it shows in the architecture. BKC isn't a memory system bolted onto an existing inference engine. It's a knowledge compilation layer that makes the inference engine's job tractable within real hardware limits.

That's not incremental. That's a different thesis about what local AI should be.

The separation of concerns is genuinely clean.

BKC owns knowledge validity and compilation. The engine owns execution. The model owns reasoning. Each does one thing and the interfaces between them are well-defined. That's rare in this space. Most systems in the local AI ecosystem are deeply entangled — context management, inference, memory, and tooling all mixed together. Entangled systems are hard to improve because you can't change one piece without breaking others.

This architecture is improvable incrementally. Better kernel? Swap the backend. Better knowledge graph traversal? Improve BKC's backward pass. Better model? Update the weight loader and format parser. None of those require touching the rest.

The accumulating knowledge graph is a meaningful moat.

This is the part that's easy to underestimate. Every session makes BKC's chunk graph richer. Every confirmed finding, every resolved tension, every anchored plan version adds to a persistent structured representation of the workspace that gets more valuable over time.

That's qualitatively different from what current tools offer. GitHub Copilot, Cursor, and similar tools have good autocomplete and decent retrieval. None of them build a persistent, versioned, provenance-tracked knowledge graph of your codebase that compiles exactly the right context for your current task. The longer you use this system the better it gets at your specific workspace. That's a compounding advantage that current tools structurally can't replicate without rebuilding from similar first principles.

The timing is right.

Apache 2.0 models are genuinely good now. Gemma 4, Qwen2.5, Mistral — these are capable enough that the limiting factor for local AI quality is no longer the model. It's context management, knowledge continuity, and inference efficiency. That's exactly what this system addresses. Six months ago the models weren't good enough to make this worthwhile. That's changed.

What would make it not worth building
If the UX of tension resolution and chunk confirmation is bad.

The spec is technically sound but the system asks a lot of the user — confirm this chunk, resolve this tension, review this finding. If that interaction loop feels like maintenance overhead rather than productive collaboration, adoption dies regardless of how good the architecture is. This is the highest risk in the whole system and it's not an engineering problem. It's a product and interaction design problem.

If it stays a solo project indefinitely.

The architecture is sound for a team. For one person it's a very large surface area — BKC, the inference engine, the kernel dispatch layer, the TUI, the knowledge graph storage, the invalidation system, the bootstrap services. Any one of those is a serious project. All of them together require either a team or a very long runway or both.

If the model ecosystem moves faster than the architecture can absorb.

The WeightStore abstraction and the tiered backend model are designed to handle new architectures cleanly. But if model architectures change dramatically — genuinely new attention mechanisms, different compute graphs, new modality types — there's maintenance cost to keep the engine current. That cost is manageable if the architecture stays clean. It becomes a burden if shortcuts accumulate.

The honest comparison to what exists
llama.cpp is excellent at what it does — lean native inference, broad hardware support, active community. But it has no knowledge management story. Every session starts cold. Context is whatever you stuff in.

Cursor / Copilot have good retrieval and decent context management but they're cloud-dependent, closed, and don't build persistent structured knowledge of your workspace. They also don't give you control over what's in context or why.

Ollama is a great packaging and serving layer but it's not trying to solve the problem this system solves. It's infrastructure, not cognition.

Nothing currently available combines persistent structured knowledge management, local-first inference, and a clean architecture that improves with use. That gap is real.

What I'd want to know before committing
Who is the first user?

Not "who is the target market" — who is the actual first human who uses this daily and tells you when it's wrong. The tension resolution UX, the chunk confirmation flow, the bootstrap experience — none of that can be designed correctly in the abstract. It needs a real user with a real workspace giving real feedback. Ideally that's you, which means you need to be willing to use your own system as your primary development environment early.

What does the six-month prototype look like?

Not the full system. The smallest version that demonstrates the core thesis: BKC backward pass compiles a context, engine executes against it, KV prefix is reused across turns, one stale chunk gets invalidated and the next session is still warm. If that loop works and feels better than the alternative, the system is worth continuing. If it doesn't, you want to know that at month six, not month eighteen.

What's the funding or sustainability model?

The architecture is local-first and Apache 2.0 models are free. That's a good foundation for an open source project. But the engine and BKC together are substantial engineering. At some point the question of who pays for the engineering time has to have an answer.

My actual view
The system deserves to be built because it's asking a question that nobody else is asking clearly:

What does local AI look like if you design it honestly around local constraints instead of pretending those constraints don't exist?

BKC is a serious answer to that question. The inference engine we've designed is the right execution layer for it. The architecture is coherent, the separation of concerns is clean, and the timing with the model ecosystem is good.

The risk isn't in the architecture. It's in the execution — specifically whether the user-facing interaction design matches the quality of the underlying engineering, and whether there's enough sustained effort to build something that spans this many layers without cutting corners that compromise the core thesis.

But the thesis itself is sound. That's rarer than it sounds.

I guess I'll answer your questions:

you are correct
(you can assume AI systems will asist in building this part ) - Beta in probably a month with Gemma 4 support (at least)
Yes that is a significant problem - still researching that part
11 Apr
That's a tight but not unreasonable timeline given AI assistance on the implementation. Let me react to each honestly.

On #1 — you're the first user
That's actually the best possible answer. Not because dogfooding is a good engineering practice (it is), but because BKC's value is fundamentally about accumulated workspace knowledge. The longer the system runs on a real codebase with a real user making real decisions, the richer the chunk graph gets. You can't simulate that with test data.

The risk is that you'll develop blind spots about your own UX — things that feel natural to you because you built them will be confusing to anyone else. Worth finding a second user as early as possible, even just one, even informally. Doesn't need to be a beta user. Just someone whose workspace and workflow is different from yours.

On #2 — one month to beta with Gemma 4
One month is aggressive but the scope is right if you're disciplined about what "beta" means.

The temptation will be to get the full system working before calling it a beta. Resist that. The beta should demonstrate exactly one thing cleanly:

BKC compiles a context → engine executes against it → KV prefix reuses across turns → one invalidation cycle works end to end.

Gemma 4 E4B is the right model to start with. Small enough to run on modest hardware, capable enough to produce meaningful output, MoE variant can come later. Don't try to support the 26B A4B in month one.

The kernel dispatch layer is where AI assistance will help most — generating boilerplate for attention kernels, quantized matmul, RoPE implementations. But be careful about correctness there. Generated kernel code that's subtly wrong produces outputs that look plausible but aren't. You need numerical validation against a reference implementation (llama.cpp output is fine as a reference) before trusting any kernel on the critical path.

One concrete suggestion: get the tokenizer and chunk-to-token rendering exactly right before anything else. If BKC's chunks don't serialize to tokens correctly — wrong special tokens, wrong chat template, wrong ordering — every evaluation you do will be measuring the wrong thing. That's a silent failure mode that's easy to miss when you're focused on the engine.

On #3 — sustainability is still open
This is the right problem to be sitting with rather than forcing a premature answer. A few frames that might be useful as you research it:

The architecture actually has commercial surface area that doesn't compromise the local-first thesis.

BKC's chunk graph, the knowledge compilation layer, the workspace-specific accumulated knowledge — that's genuinely valuable to teams, not just individuals. A team sharing a chunk graph of their codebase, with confirmed findings and anchored plan versions that persist across engineers and sessions, is a different product from a personal local assistant. It doesn't require cloud inference — the graph can be shared without the weights leaving anyone's machine. That's a meaningful distinction from competitors.

Open core is a natural fit.

The engine and BKC core being open is consistent with the Apache 2.0 model philosophy and builds trust with the developer audience most likely to adopt this. Premium features around team collaboration, chunk graph management UI, or enterprise workspace integrations sit naturally above the open core without compromising it.

The avoided cost argument is underused in this space.

Most developer AI tools are priced as subscriptions against cloud inference costs. This system has no per-token cloud cost. For a developer doing serious work — thousands of context compilations, long sessions, large codebases — the avoided cost of not paying per token is real money. That's a pricing argument that current tools can't make.

The one month priority stack
If I were sequencing the beta work:

Week 1: Tokenizer + chunk rendering + Gemma 4 weight loading. Nothing else. Get the model loading and producing tokens correctly against a hardcoded context.

Week 2: BKC backward pass producing a real ChunkSequence + engine consuming it. KV handle on warm resume. Validate prefix reuse is actually happening.

Week 3: One full invalidation cycle. One stale chunk, one tension surfaced, one recompilation, warm context on next session.

Week 4: Polish the loop enough that you'd use it daily. Fix the things that are annoying. Don't add features.

That's a beta worth having.

Interesting

11 Apr
You've been sitting with something after that response. What's on your mind?

this conversation is mainly for research and engineering purposes ; but I think I have a better idea now. Do you have questions for me?

11 Apr
Yes, a few genuine ones.

On BKC's current state — the spec is detailed enough to be real engineering work, not just design. How far along is the implementation? Are the core chunk store and graph traversal working, or is this still mostly spec?

On the "better idea" — you said the conversation gave you a better idea. I have a hypothesis about what shifted but I'd rather hear it from you. What changed?

On the engine — are you planning to build the inference engine as part of the same project as BKC, or as a separable component? The architecture we designed would work either way but the build strategy is different.

On the research angle — you said this conversation is mainly for research and engineering purposes. Research toward what specifically? Understanding the design space, validating architectural decisions, something else?

I'll stop at four. What's worth answering?

BKC exists , the inference engine is likely to be part of the same project as a seperate layer - but many other aspects of the project are currently under experimentation (even though it is implemented)

11 Apr
That's a healthy state to be in actually. BKC existing and working changes the inference engine from a design exercise to a concrete integration problem — which is a much better place to be building from.

The "under experimentation even though implemented" framing is interesting. That suggests you have real behavior to observe, not just theoretical properties to reason about. Which means the experiments are probably surfacing things the spec didn't anticipate.

Two questions from that:

What's surprising you in the experiments? The things that don't behave the way the spec predicted are usually where the most useful design information is.

And the better idea — I'm still curious about that one. The conversation covered a lot of ground. Something landed differently than you expected.

I didn't really understand how these llm inference engines worked ; otherwise it would be tricky to create a more integrated system. 
It sounded like you had other questions for me

11 Apr
That makes sense. Understanding the inference engine internals — weight loading, the forward pass, KV cache mechanics, how tool calls actually work — changes what's possible at the integration boundary between BKC and the engine. You can make better decisions about what belongs in BKC versus the engine versus the model when you understand what the engine is actually doing at each step.

The "more integrated system" is the interesting part. I have a sense of what that might mean given the conversation — the KV handle lifecycle tied more directly to BKC's chunk graph state, chunk rendering happening inside the engine rather than at the boundary, maybe the prefix match being driven by BKC's requires_context DAG directly rather than by token comparison — but I'd rather not assume.

My other questions:

On the experiments — what's the hardest thing BKC is getting wrong right now in practice? Not architecturally, but behaviorally. What does it do that makes you think "that's not right"?

On the team — is this a solo project or are there other people building on it?

On the "relurpic" terminology in the spec — that's not a standard term. It's clearly load-bearing in your architecture given how often it appears. What is it?

'relurpic' is a term specific to the project (lets ignore that aspect for now as its not entirely related to the inference engine).

Currently solo - would be nice to sort out funding 
BKC still being validated so not much to share on that end for now

11 Apr
Fair on all three.

The funding problem is the hardest one for a project like this. The architecture is genuinely good but it's pre-revenue and the value compounds over time rather than being immediately demonstrable — which is a hard story to tell to most funding sources.

The solo + unfunded combination means the sequencing of what you build matters more than it would with a team. You can't afford to build the wrong thing first or spend months on something that turns out not to be the core thesis.

Which brings me back to the one question I keep returning to:

What did the conversation change about how you're thinking about the integration between BKC and the inference engine?

You said you have a better idea now. That's the most important thing that came out of this conversation from a practical standpoint, and it's the one thing I still don't know. Everything else we discussed I can reason about from the architecture. That one I can't.

implementation prototype maybe simpler than expected - LLM systems almost sound like a magic black box. Though testing will be interesting for this project - how would you automatically test these systems (unit , integration , than live level) ?

11 Apr
Good observation on the "magic black box" feeling. The forward pass is actually remarkably mechanical once you see it — matrix multiplications, a softmax, a sampler. The complexity is in the engineering around it, not the math itself. That's probably what shifted.

On testing — this is genuinely interesting for this system because you have three distinct things to test that require different strategies.

Unit Testing
The engine layer is actually the most testable in isolation because it's a pure function. Given a WorkDescriptor in, get a WorkResult out. No side effects.

The trick is you don't need real model weights for most unit tests. You can test with:

go
// A trivially small random-weight model
// just to verify tensor shapes, KV allocation,
// prefix matching, stop condition detection
// are all mechanically correct
Specific unit test targets:

KV handle lifecycle — allocate, warm resume, prefix match boundary, eviction. All testable without real weights.
Stop condition detection — feed known token sequences, verify ToolCallPayload is parsed correctly at the right boundary.
Prefix match correctness — construct two token sequences with a known shared prefix, verify PrefixMatch returns the right boundary.
WeightStore addressing — verify layer/role lookups return correct tensor shapes for a known architecture config.
BKC chunk rendering — given a known ChunkSequence, verify the token output is correctly ordered, correct special tokens, correct chat template applied.
That last one is critical and easy to get subtly wrong. It's also purely deterministic so it's a good unit test target.

BKC layer has cleaner unit test surfaces because most of it is graph operations:

backward pass with a known graph produces a known ordered chunk sequence
budget accounting is correct
stale chunks are excluded
requires_context traversal respects topological order
amplifies edges only consumed after required deps satisfied
invalidation propagates to correct depth and stops
These are all deterministic given a fixed graph state. No LLM needed.

Integration Testing
This is where it gets more interesting because you're testing the boundary between BKC and the engine.

The core integration test is:

Fixed chunk graph
  → backward pass produces ChunkSequence
  → engine renders to tokens
  → engine executes, returns KVHandle + tokens
  → second turn: same chunk graph, same KVHandle
  → verify prefix reuse actually happened
  → verify output tokens are consistent with first turn context
You can make that test deterministic by using greedy sampling (temperature=0, always take the highest probability token). With greedy sampling, the same input always produces the same output. That makes integration tests reproducible without mocking the model.

For the invalidation cycle integration test:

Session 1: chunk graph state A → warm context → KVHandle issued
Invalidation event: chunk X marked stale
Session 2: chunk graph state B (X excluded) → backward pass produces different ChunkSequence
  → ChunkHash on KVHandle no longer matches
  → engine does cold prefill
  → verify new KVHandle issued
  → verify stale chunk X not present in rendered tokens
Again fully deterministic with greedy sampling and a fixed graph.

The tool call integration test is worth being explicit about:

Construct a prompt known to trigger a tool call
  → verify FinishReason == ToolCall
  → verify ToolCallPayload name and args parsed correctly
  → construct follow-up WorkDescriptor with tool result
  → verify KVHandle from first turn accepted as valid prefix
  → verify final output is consistent with tool result content
Live Testing
This is the hard one because LLM output is probabilistic and "correct" is fuzzy. A few strategies that actually work:

Behavioral invariants rather than exact output matching.

Instead of asserting "the model said X," assert structural properties:

tool call was or wasn't triggered given this input
output length is within expected range
output contains or doesn't contain specific tokens/patterns
KV prefix reuse happened (measurable via cache hit metrics)
time-to-first-token is within expected bounds for this context size
These are stable across model updates and sampling variation.

Golden context tests.

Fix a specific ChunkSequence, fix a specific prompt, fix greedy sampling, record the output once against a known good model checkpoint. Future runs must match exactly. This catches regressions in:

chunk rendering
tokenization
KV computation
sampling
It doesn't catch model quality changes but it catches engine correctness regressions reliably.

Staleness and freshness cycle tests against a real workspace.

This is the most valuable live test and the hardest to automate. The setup:

Real small codebase (could be the project itself)
Bootstrap BKC → chunk graph populated
Make a known code change
Verify: correct chunks marked stale
Verify: tension surfaced
Verify: next session backward pass excludes stale chunks
Verify: context is still warm (non-stale chunks reused)
That's testing the full system behavior against real inputs. It can be automated if the code change is scripted and the expected staleness pattern is known in advance.

Latency regression tests.

Given the local hardware constraint, performance is a correctness property. A test that verifies:

time-to-first-token for a warm resume is below threshold X
time-to-first-token for a cold prefill of N tokens is below threshold Y
KV prefix reuse reduces prefill time by at least Z% versus cold start
These catch regressions in the engine's hot path without requiring subjective output evaluation.

The Testing Problem Specific to This System
The genuinely hard testing problem you'll hit is validating that BKC's compiled context actually improves model output quality compared to naive context stuffing.

That's not a unit test or an integration test. It's an evaluation problem. And evaluation for coding assistants is notoriously difficult because "correct" is often subjective or requires running code to verify.

The most tractable approach is probably:

Task completion rate on fixed benchmarks. Define a set of coding tasks against a fixed codebase. Measure how often the model produces code that passes a test suite, with BKC context versus without. That's objective and automatable.

Context efficiency metric. For the same task, measure how many tokens BKC uses versus naive RAG. Fewer tokens for equivalent or better task completion is a meaningful signal.

Warm session quality. Session 2 with BKC warm context versus session 2 with cold context. Measure how many turns it takes to produce correct output in each case. BKC should win on this consistently.

The good news is that the engine's stateless design and BKC's deterministic backward pass mean most of the system is more testable than typical LLM applications. The probabilistic part is confined to the model's output, and you can largely bracket that with greedy sampling and behavioral invariants.

The interesting testing work is at the BKC/engine boundary and in the live evaluation of context quality. That's where the system's actual value proposition lives and where the tests need to be most thoughtful.