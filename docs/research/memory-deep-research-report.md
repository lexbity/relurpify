# Cutting-edge External Memory Systems for Large Language Models

## Executive summary

External memory systems let an LLM “know” more than fits in its weights or context window by *reading* from (and sometimes *writing to*) an external store at run time. The mainstream pattern in production is still **retrieval‑augmented generation (RAG)**—retrieve relevant chunks from a corpus and inject them into the prompt—because it is modular, relatively cheap to iterate on, and can be updated without retraining the LLM. This is the core idea formalised in early RAG work that framed retrieval as “non‑parametric memory” alongside parametric model weights. citeturn0search0turn0search8

Research has pushed beyond “classic RAG” in three big directions: (1) **retrieval-integrated training/inference** where retrieval is part of the model’s computation graph or generation loop (e.g., REALM, RETRO, Atlas), (2) **latent/activation memories** that store internal representations or key‑value pairs for kNN lookups (e.g., kNN‑LM, Memorizing Transformers), and (3) **long-context + compression** mechanisms that aim to make the model itself handle far longer histories with bounded compute (e.g., Infini-attention, Titans). citeturn0search1turn0search10turn0search2turn15search7turn4search2turn4search5turn8search1turn8search0

In parallel, “agent” stacks have popularised **episodic, hierarchical, and tool-augmented memory**: store interaction traces, summarise them, link them, and retrieve them adaptively during planning (e.g., Generative Agents, MemGPT/Letta, A‑MEM, MemGen). These systems treat memory as *state* plus *policies* (what to write, what to keep, what to retrieve), not just a database query. citeturn9search0turn0search7turn16search0turn16search17turn8search7

Assumptions: you did not specify the target LLM, corpus size, latency budget, or deployment setting, so this report evaluates designs in a model-agnostic way (hosted or self-hosted, small to large scale). Where trade-offs depend heavily on scale (e.g., millions vs billions of vectors), the report makes that dependency explicit rather than assuming a single regime. citeturn1search13turn2search19turn14search4

## Scope and conceptual map

“External memory” in LLM systems typically means **non-parametric state** stored outside the model weights that can be accessed at inference time—documents, embeddings, key-value records, graphs, caches, tool outputs, and interaction histories. In many papers, it is contrasted with *parametric memory* (knowledge baked into weights). citeturn0search0turn0search1turn4search2

It helps to separate four layers that often get conflated:

1. **Knowledge stores** (durable): corpora, databases, logs, code repos, wikis, tickets—usually persisted and versioned.
2. **Retrieval indexes** (serving structures): vector indexes (HNSW/IVF/PQ), sparse inverted indexes (BM25), graph indexes, or hybrid fusions. citeturn2search0turn1search16turn1search6turn10search3
3. **Context construction** (prompt assembly): chunking, filtering, re-ranking, deduping, citation formatting, and budget allocation. citeturn9search2turn9search6turn15search1
4. **Compute-time memory** (ephemeral): KV caches for decoding, semantic caches for repeated queries, and short-lived tool results. citeturn13search0turn13search2

A key practical framing: most production “memory” failures are *retrieval and context construction failures*, not model failures—wrong chunks, stale chunks, missing metadata, or blown context budgets—so the engineering focus is often on pipelines, indexing, and evaluation rather than exotic neural memory modules. citeturn11search2turn11search0turn0search0

## Architectures and paradigms

### Retrieval-augmented generation as non-parametric memory

Classic RAG combines (a) a **retriever** over a corpus (often vector-indexed) and (b) a **generator** that conditions on retrieved passages. The canonical paper compared two formulations: conditioning on a fixed set of retrieved passages for the whole generation vs retrieving per token. citeturn0search0turn0search8

In practice, “production RAG” tends to look like:
- Retrieve top‑k candidate chunks (dense, sparse, or hybrid),
- Optionally re-rank (cross-encoder or LLM),
- Insert the best few chunks into a prompt template,
- Generate a response with citations and guardrails.

Framework docs (LangChain, LlamaIndex, Haystack) codify this as reusable components—document loaders, chunkers, embedders, vector stores, retrievers, and query engines. citeturn9search1turn9search2turn9search3

### Retrieval-integrated training and retrieval controllers

Several research lines push retrieval *into* the model’s learning loop or control policy:

- **REALM** trains a latent retriever during LM pretraining, backpropagating through a retrieval step so the model learns when/what to retrieve from a large corpus (e.g., Wikipedia). citeturn0search1turn0search5turn0search17  
- **Atlas** jointly pretrains a retriever and a generative model, reporting strong few-shot performance on knowledge-intensive tasks and benchmarks (including KILT tasks). citeturn15search7turn15search15turn11search1  
- **RETRO** conditions generation on retrieved chunks from a massive token database using a retrieval-augmented Transformer design, showing that retrieval can substitute for parameter count in some regimes. citeturn0search2turn0search10  
- **REPLUG** is notable because it treats the LLM as a black box: retrieve docs, prepend them, and train/tune the retriever (not the LM) to match the LM’s preferences. citeturn6search2turn6search6  
- **Self‑RAG** adds an explicit *controller-ish* mechanism: the model learns to retrieve on demand and uses “reflection tokens” to critique and correct generation conditioned on retrieved evidence. citeturn6search3turn6search7turn6search11

A practical reading: “retrieval controllers” are any mechanism that **allocates retrieval budget dynamically** (retrieve or not, how many, when to stop, which sources) instead of blindly stuffing top‑k. Self‑RAG is an explicit example; Atlas/REALM bake it into training; many production stacks approximate it with heuristics (“ask a clarifying question if retrieval confidence is low”, “increase k if entropy is high”, etc.). citeturn6search7turn15search7turn0search1

### Latent / activation memory via kNN lookups

A different paradigm stores *representations* rather than text:

- **kNN‑LM** stores (key, value) pairs where keys are LM hidden states and values are next-token distributions/tokens from a datastore; inference interpolates the base LM with a kNN distribution retrieved by ANN search. citeturn4search1turn4search4turn4search18  
- **Memorizing Transformers** store “recent” internal representations and query them with approximate kNN at inference time, improving language modelling and allowing test-time uptake of new symbols in some benchmarks. citeturn4search2turn4search5

These works are “external memory” in a very literal systems sense: the datastore can be swapped for domain adaptation, scaled independently, and updated without gradient updates to the base model. citeturn4search1turn4search2

### External differentiable memory and memory-augmented neural nets

Before the LLM era, “external memory” often meant a differentiable read/write matrix:

- **Neural Turing Machines** introduced end-to-end differentiable read/write over an external memory matrix, enabling learning of algorithmic tasks (copy, sort, associative recall). citeturn5search0turn5search4  
- **Differentiable Neural Computer** extended this idea with richer addressing and memory usage mechanisms, explicitly analogising the memory to RAM. citeturn5search1turn5search13  
- **End‑to‑End Memory Networks** used attention over a memory of sentences with multiple “hops”, forecasting later retrieval+reasoning ideas. citeturn5search2turn5search6

Why these aren’t the default in modern LLM applications: differentiable memory is harder to scale, harder to operate reliably, and often less interpretable/debuggable than retrieval over explicit corpora—though recent “test-time memory modules” (see Titans) are arguably revisiting the idea with a modern training story. citeturn8search0turn5search1turn0search0

### Long-context Transformers, compression, and hierarchical memory

Long-context modelling can be seen as an *internal* memory extension, but it competes directly with external memory: if the model can attend over more tokens cheaply, you need retrieval less often.

Key families:

- **Recurrence / cached states:** Transformer‑XL extends dependence beyond a fixed context by segment-level recurrence and caching hidden states. citeturn5search3turn5search7  
- **Compression:** Compressive Transformer explicitly compresses older memories to keep long-range signals with bounded compute. citeturn7search0turn7search4  
- **Sparse attention:** Longformer and BigBird reduce attention complexity (often to near-linear) by structured sparse patterns, enabling far longer inputs than vanilla quadratic attention. citeturn7search1turn7search13turn7search6turn7search10  
- **Memory tokens / hierarchical segments:** Recurrent Memory Transformer adds special memory tokens and recurrence, designed to pass information between segments with minimal changes to the base Transformer. citeturn7search3turn7search7  
- **Streaming “infinite” context with bounded resources:** Infini-attention proposes a compressive memory integrated into attention to allow streaming inference and effectively unbounded context. citeturn8search1turn8search4  
- **Test-time trainable long-term memory modules:** Titans proposes a neural long-term memory module that “learns to memorise at test time” and reports scaling to very large context lengths on needle-in-haystack style tasks. citeturn8search0turn8search3

From an external-memory engineering perspective, these approaches shift the bottleneck from retrieval/indexing to **serving and KV-cache memory management**, because long contexts make decoding memory dominate. That’s why systems work like vLLM’s PagedAttention matters: it targets fragmentation and sharing in KV cache to raise throughput. citeturn13search0turn13search8

### Episodic, agentic, and tool-augmented memory

Agent architectures treat memory as *experience* plus *reflection*:

- **Generative Agents** store a stream of natural-language “memories”, periodically summarise/reflect to form higher-level beliefs, and retrieve by relevance/recency/importance for planning. citeturn9search0turn9search4  
- **MemGPT** frames the agent like an OS managing memory tiers (“working context” vs external store), with policies for paging information in/out of the context window. The project has since evolved into **Letta (formerly MemGPT)**, a platform for stateful agents. citeturn0search7turn16search0  
- **A‑MEM** proposes an “agentic memory” inspired by Zettelkasten: notes with structured attributes, dynamic linking, and memory evolution as new memories arrive. citeturn16search17turn0search15turn16search1  
- **MemGen** (recent) focuses on *generative latent memory* for self-evolving agents, positioning memory as a latent substrate recalled/augmented through reasoning cycles. citeturn8search7turn8search5  
- **Tool-augmented memory policies:** ReAct interleaves “reasoning traces” with actions that query external sources (Wikipedia API, etc.), explicitly targeting hallucination/error propagation by grounding through tool interaction. Toolformer trains models to decide which APIs to call and how to use results. citeturn6search0turn6search12turn6search1turn6search5

These systems converge on a common idea: an LLM’s limited context is a scarce resource, so “memory” is really **a control problem**: what to write, what to summarise, what to link, what to retrieve, and what to forget. citeturn0search7turn16search17turn7search0

### Timeline of representative milestones

```mermaid
gantt
dateFormat  YYYY-MM-DD
title External memory systems for LLMs and predecessors

section Differentiable / neural memory roots
Neural Turing Machines (NTM)           :milestone, 2014-10-20, 1d
End-to-End Memory Networks             :milestone, 2015-03-31, 1d
Differentiable Neural Computer (DNC)   :milestone, 2016-10-12, 1d

section Long-context and compression
Transformer-XL                         :milestone, 2019-01-09, 1d
Compressive Transformer                :milestone, 2019-11-13, 1d
Longformer                             :milestone, 2020-04-10, 1d
BigBird                                :milestone, 2020-07-28, 1d

section Retrieval-augmented LMs
REALM                                  :milestone, 2020-02-10, 1d
RAG                                    :milestone, 2020-05-22, 1d
RETRO                                  :milestone, 2021-12-08, 1d
Atlas                                  :milestone, 2022-08-05, 1d

section Activation / kNN memory
kNN-LM                                 :milestone, 2019-11-01, 1d
Memorizing Transformers                :milestone, 2022-03-16, 1d

section Agentic / tool memory
ReAct                                  :milestone, 2022-10-06, 1d
Toolformer                             :milestone, 2023-02-09, 1d
Generative Agents                       :milestone, 2023-04-07, 1d
MemGPT                                  :milestone, 2023-10-12, 1d
Self-RAG                               :milestone, 2023-10-17, 1d
GraphRAG                               :milestone, 2024-04-24, 1d
Infini-attention                        :milestone, 2024-04-10, 1d
Titans                                 :milestone, 2024-12-31, 1d
A-MEM                                  :milestone, 2025-02-17, 1d
MemGen                                 :milestone, 2025-??-??, 1d
```

Dates correspond to the first public arXiv/proceedings appearances for the cited works. citeturn5search0turn5search2turn5search1turn5search3turn7search0turn7search1turn7search6turn0search1turn0search0turn0search2turn15search3turn4search1turn4search2turn6search0turn6search1turn9search0turn0search7turn6search3turn15search0turn8search1turn8search0turn16search17turn8search7

## Implementation deep dive

This section focuses on how external memory systems are actually built: data structures, indexing, retrieval quality, consistency, and latency.

### Canonical data model: documents, chunks, embeddings, metadata

Most RAG systems converge on a few core entities:

- **Document**: logical source (file, web page, ticket, wiki article) with stable ID, source URI, permissions, timestamps, and version.
- **Chunk (node)**: a contiguous span (token window, sentence group, semantic segment) with `doc_id`, `chunk_id`, text, offsets, and optional parent relationships (for hierarchical retrieval).
- **Embedding**: vector representation of the chunk (and sometimes separate vectors for title, summary, or per-sentence/per-token embeddings).
- **Metadata payload**: filterable fields (tenant, permissions, language, product area, created_at, updated_at, tags, “freshness”), plus provenance (source, page, line numbers).

Framework docs explicitly recommend (or at least expose APIs for) chunking and attaching metadata, because retrieval quality and access control depend on it. citeturn9search2turn9search6turn9search22

A practical insight: **metadata is part of retrieval**, not an afterthought. It enables:
- hard filtering (ACLs, tenant isolation),
- soft boosting (freshness/authority),
- auditability (citations),
- and operational policies (TTL, reindex scheduling). citeturn10search4turn10search0turn1search6

### Chunking strategy: it’s a systems parameter, not a preprocessing detail

Chunking exists because both embedding models and LLMs have context limits, and because retrieval works better when chunks are “about one thing”. LlamaIndex documentation explicitly frames chunking as a step to deal with context window limits and offers multiple modes (token, sentence, semantic). citeturn9search6turn9search2

Common production patterns:

- **Fixed token windows with overlap** (e.g., 256–1,024 tokens, 10–20% overlap): reliable, fast, but can split concepts.
- **Structure-aware chunking** (headings/sections/code blocks): better semantic integrity.
- **Semantic chunking** (split where embedding similarity drops): can improve coherence but is more expensive and depends on embedding stability.

Chunk size also couples to your retriever: dense retrievers often like “passage sized” text (as in DPR/RAG), while late-interaction models (ColBERT-style) can work with longer spans but require different indexing. citeturn12search0turn0search0turn12search5

### Embedding models: dual encoders, late interaction, and instruction-aware embeddings

Retrieval quality is bounded by embedding quality.

- **Dual-encoder dense retrieval**: DPR is a widely cited baseline where a query encoder and passage encoder produce vectors; nearest-neighbour search yields candidates. citeturn12search0turn12search8  
- **General-purpose embedding families**: E5 reports strong retrieval transfer and explicitly evaluates on BEIR/MTEB, positioning itself for broad retrieval use. citeturn12search2turn12search14turn11search0  
- **Late interaction**: ColBERT encodes query and document into multiple vectors and matches via late interaction, improving quality at higher compute and different index design. citeturn12search5turn12search1

Operationally, embedding choice impacts:
- vector dimensionality (storage and bandwidth),
- whether you can run embeddings on GPU (latency),
- whether you need query rewriting (HyDE) for domain mismatch,
- and whether you can do multi-vector or hybrid sparse+dense retrieval. citeturn15search1turn1search6turn12search5

### Vector search algorithms and indexes

At scale, exact search (scan all vectors) is too slow, so most systems use **approximate nearest neighbour (ANN)** indexes.

#### Graph-based ANN: HNSW and DiskANN

- **HNSW** builds a hierarchical proximity graph and performs greedy/beam-like search; it is widely used in vector databases and has strong recall/latency trade-offs. citeturn2search0turn2search4turn1search16turn10search11  
- **DiskANN** targets billion-scale search with much smaller RAM by storing index structures on SSD while retaining high recall, which matters when your corpus doesn’t fit economically in memory. citeturn2search19turn2search3turn2search7

A key engineering trade: graph indexes often give great recall quickly, but updates and deletes can be operationally tricky at very high scale; disk-based designs reduce RAM pressure but add I/O complexity and different tail-latency behaviour. citeturn2search19turn10search11turn10search7

#### Inverted-file and quantisation: IVF, PQ, ScaNN

- **FAISS** documents composite indexes like IVF and PQ and provides guidance on choosing index configurations (including how much training data you need for IVF clustering). It also supports GPU implementations for some algorithms. citeturn1search13turn1search10turn1search4  
- **Product Quantization (PQ)** (classic) compresses vectors into short codes to reduce memory and speed distance estimates; it underpins many “IVF_PQ” style indexes (and variants like OPQ). citeturn3search11turn3search20  
- **ScaNN** (Google) focuses on efficient maximum inner-product search with quantisation; the ICML paper on anisotropic vector quantisation describes why score-aware quantisation can improve retrieval quality. citeturn3search0turn2search2turn3search4

A useful mental model:

- If you’re under ~1M vectors and can keep things in RAM, HNSW or flat may be simplest.
- Into tens/hundreds of millions, IVF/PQ style compression is often needed for memory cost.
- At billion scale with limited RAM, disk-based (DiskANN-like) approaches become compelling.

FAISS’ own docs make this kind of scale-dependent guidance explicit. citeturn1search4turn1search13turn2search19

### Vector databases: sharding, replication, and operational guarantees

Vector DBs industrialise ANN retrieval by adding persistence, metadata filtering, horizontal scaling, and ops tooling.

Representative operational designs (from official docs):

- **Milvus** is a distributed architecture that separates compute and storage and has explicit components for ingestion/querying; the architecture overview describes roles like Query Node and Data Node, and a “Streaming Node” for shard-level consistency and recovery. citeturn14search4turn14search0turn14search8  
- **Qdrant** supports sharding and shard replication; its distributed deployment docs describe replicating shards across nodes to increase reliability. citeturn14search1turn14search5turn14search9  
- **Weaviate** documents replication and sharding as separate concerns and describes how shards/replica sets affect availability and throughput. citeturn14search3turn14search7turn14search19  
- **Pinecone**’s docs describe scaling via pods and replicas (pods add resources; replicas add throughput), and they caution about batching deletions to avoid latency spikes in shared read/write compute. citeturn14search2turn14search18turn1search2

Two operational points that matter for “memory” semantics:

1. **Update visibility / consistency:** Some systems expose near-real-time upserts, but index structures may need background compaction or rebuilds, and replicated/sharded systems may provide eventual consistency depending on configuration. (Milvus explicitly discusses streaming and shard-level guarantees; Qdrant/Weaviate discuss replication mechanics.) citeturn14search0turn14search1turn14search3  
2. **Filtering with ANN:** Real deployments usually require metadata filtering (tenant, ACL, time). Qdrant explicitly discusses payload indexes and “filterable HNSW” approaches, while Redis/Weaviate/Elastic also document vector+metadata querying. citeturn10search4turn10search8turn13search7turn1search6turn10search7

### Freshness, TTL, and memory evolution

“Freshness” is a core reason to use external memory: you can update the corpus without retraining the model. RAG papers explicitly motivate this modularity. citeturn0search0turn0search1

In implementation, freshness is usually enforced by **metadata + policies**, not magic:

- Store `created_at`, `updated_at`, `source_version`, and optionally `expires_at`.
- Filter or boost by time at retrieval (e.g., last 30 days).
- Periodically delete/archive expired vectors (batch deletions to avoid latency spikes in some managed systems). citeturn14search18turn10search0turn1search6

For evolving “agent memories” (A‑MEM, Generative Agents), freshness interacts with *reflection/summarisation*: older episodes may be summarised into stable “belief” nodes while raw logs are retained for audit. citeturn16search17turn9search0turn0search15

### Latency optimisation: multi-stage retrieval, batching, and caching

End-to-end latency is often dominated by a small set of expensive steps: embedding, ANN search, re-ranking, and LLM decoding.

Common high-impact strategies, grounded in docs/papers:

- **Hybrid retrieval (dense + BM25)**: Weaviate documents hybrid search that combines vector and BM25 scores; this often improves recall on exact terms (IDs, error codes) without sacrificing semantic matching. citeturn1search6turn1search3  
- **Two-stage funnels**: retrieve a broader top‑k cheaply, then re-rank a smaller subset; BEIR highlights that re-ranking/late-interaction models can improve zero-shot performance but at higher cost. citeturn11search0turn12search5  
- **Batching embeddings**: batch query embeddings on GPU; batch upserts in index builds; avoid per-request model initialisation overhead (framework-level concern). citeturn1search13turn14search4  
- **GPU-accelerated vector ops when it fits**: FAISS supports GPU for some algorithms; Milvus exposes GPU index types like GPU_IVF_PQ explicitly designed for GPU environments. citeturn1search13turn1search5  
- **Serving-time memory management for decoding**: vLLM’s PagedAttention targets KV cache fragmentation and enables larger effective batches and cache sharing to increase throughput. citeturn13search0turn13search8  
- **Semantic caching**: GPTCache positions itself as a semantic cache to reduce repeated LLM calls; Redis also documents vector search in Redis for semantic retrieval patterns. citeturn13search2turn13search10turn13search7

### Reference pipeline: data flow in an external-memory LLM system

```mermaid
flowchart LR
  subgraph Ingestion["Ingestion / Memory Write Path"]
    A[Sources\nfiles, wikis, DB rows, events] --> B[Normalise + parse\nclean, split, dedupe]
    B --> C[Chunker\n(token/sentence/semantic)]
    C --> D[Embedder\nGPU batch if possible]
    D --> E[(Vector Index / DB)]
    C --> F[(Doc store)\nraw text + provenance]
    B --> G[(Metadata store)\nACLs, timestamps, versions]
  end

  subgraph Query["Query / Memory Read Path"]
    Q[User query] --> Q1[Query rewrite?\nHyDE, decomposition]
    Q1 --> Q2[Query embedder]
    Q2 --> R1[Retriever\nANN / hybrid]
    R1 --> R2[Filter + rerank\nmetadata + cross-encoder]
    R2 --> P[Prompt builder\nbudget, citations]
    P --> L[LLM generate]
    L --> O[Answer + citations]
  end

  subgraph Control["Controllers + Caches"]
    K1[(KV cache / prefix cache)] --- L
    K2[(Semantic cache)] --- Q
    T[Tools\nsearch, SQL, APIs] <--> L
    M[Memory policy\nwhat to store/summarise] --> Ingestion
  end

  E --> R1
  F --> P
  G --> R2
```

This is the “typical” architecture implied across RAG frameworks and tool-augmented agent papers: explicit ingestion, explicit retrieval, and explicit prompt assembly with optional tool calls and caching layers. citeturn9search2turn9search3turn6search0turn13search0turn13search2turn15search1

### Implementation examples (patterns, not full apps)

#### Minimal RAG pipeline (pseudocode)

```python
def answer(question: str) -> str:
    q_vec = embed_query(question)              # batch on GPU if available
    candidates = vectordb.search(q_vec, k=50, filters={"tenant": "acme"})
    reranked = rerank(question, candidates)    # cross-encoder or lightweight LLM
    context = pack_context(reranked[:6], token_budget=2500, with_citations=True)
    prompt = render_prompt(question, context)

    return llm_generate(prompt, max_tokens=600)
```

Why this shape persists: it cleanly separates retriever quality, context construction, and generator behaviour, which makes iteration and evaluation faster (one module at a time). citeturn0search0turn9search1turn11search2

#### Vector DB integration with metadata and upserts (illustrative)

```python
def upsert_chunks(doc_id, chunks, namespace):
    rows = []
    for chunk in chunks:
        rows.append({
            "id": f"{doc_id}:{chunk.chunk_id}",
            "vector": embed_doc(chunk.text),
            "payload": {
                "doc_id": doc_id,
                "chunk_id": chunk.chunk_id,
                "created_at": chunk.created_at,
                "source": chunk.source,
                "acl": chunk.acl,
                "lang": "en",
            }
        })
    vectordb.upsert(rows, namespace=namespace)   # durability + index update
```

Most modern vector DBs support some notion of payload/metadata for filtering and audit, and distributed systems add sharding/replication semantics behind this API surface. citeturn10search4turn14search1turn14search3turn14search4

#### Streaming / online memory updates (append + refresh policy)

```python
def ingest_event(event):
    # event may be: support ticket update, wiki edit, new log entry, code diff
    chunk_texts = chunk(event.text)
    vectors = embed_batch(chunk_texts)

    vectordb.upsert_many([
        {"id": event.chunk_id(i), "vector": vectors[i],
         "payload": {"event_time": event.time, "type": event.type, "version": event.version}}
        for i in range(len(chunk_texts))
    ])

    # Optional: decay/TTL policy (soft-delete or mark stale)
    mark_superseded_versions(event.entity_id, keep_latest=True)
```

This “append + supersede” pattern is common because many ANN indexes handle incremental inserts well, while arbitrary deletes can be expensive; so systems often implement *logical deletion* (filter out old versions) and schedule physical compaction. citeturn14search0turn2search0turn10search7

#### Cache eviction policy for semantic caches (LRU + TTL hybrid)

```python
class SemanticCache:
    def __init__(self, max_items, ttl_seconds):
        self.max_items = max_items
        self.ttl = ttl_seconds
        self.lru = OrderedDict()  # key -> (value, expires_at)

    def get(self, key):
        if key not in self.lru:
            return None
        value, exp = self.lru[key]
        if now() > exp:
            del self.lru[key]
            return None
        self.lru.move_to_end(key)
        return value

    def put(self, key, value):
        self.lru[key] = (value, now() + self.ttl)
        self.lru.move_to_end(key)
        while len(self.lru) > self.max_items:
            self.lru.popitem(last=False)  # evict LRU
```

Semantic caches (e.g., GPTCache) add an embedding similarity layer so “near-duplicates” hit the cache, but the eviction logic still tends to be LRU/LFU/TTL variants in practice. citeturn13search2turn13search10

## Evaluation metrics and benchmarks

### Retrieval metrics: recall@k, MRR, nDCG, and calibration

For the retrieval subsystem, core metrics include:

- **Recall@k**: fraction of queries where the correct passage is in top‑k.
- **MRR** (mean reciprocal rank): rewards putting the first correct item higher.
- **nDCG**: handles graded relevance and multiple relevant items.

BEIR explicitly evaluates diverse model families (lexical, dense, late-interaction, re-rankers) across heterogeneous datasets to probe out-of-distribution generalisation and cost trade-offs, providing a retrieval-centric benchmark lens. citeturn11search0turn11search4

KILT is especially relevant to “external memory” LMs because tasks are grounded in a shared Wikipedia snapshot and the benchmark is positioned to accelerate research into memory architectures and retriever+generator components. citeturn11search1turn11search13

### End-to-end metrics: accuracy, faithfulness, citation quality, hallucination reduction

Classic QA metrics (Exact Match, F1) are still used for RAG evaluations (RAG, DPR, Atlas), but external-memory systems add additional dimensions:

- **Faithfulness / groundedness**: does the answer follow from retrieved context?
- **Citation precision**: are citations actually supporting the claim?
- **Hallucination rate**: how often the model asserts unsupported facts.

RAG’s original motivation explicitly includes improving factuality/specificity by conditioning on retrieved text, and Self‑RAG explicitly targets factuality and citation accuracy using retrieval-on-demand + reflection. citeturn0search0turn6search7

Automated evaluation frameworks like **RAGAS** propose reference-free metrics to separately score retrieval quality, faithfulness, and generation quality, aiming to shorten iteration cycles when human labels are scarce. citeturn11search2turn11search6

### Systems metrics: latency, throughput, cost, tail behaviour

External memory adds hard systems constraints:

- **p50/p95/p99 latency**: retrieval adds network hops; re-ranking adds compute; long contexts add decoding time.
- **Throughput (QPS)**: batching limits, KV-cache memory limits, and vector DB concurrency become bottlenecks.
- **Cost per query**: embedding inference + vector DB + LLM tokens + caching strategy.

On the decoding side, vLLM reports throughput improvements via KV-cache paging and sharing, directly targeting the memory bottleneck of LLM serving. citeturn13search0turn13search12

On the retrieval side, ANN algorithms explicitly trade accuracy for speed; Elastic’s kNN docs spell out that HNSW-based approximate kNN sacrifices exactness for speed. citeturn10search11turn2search0

A rigorous evaluation setup usually needs:
- a retrieval benchmark slice (BEIR-like) to tune retriever/index,
- a task benchmark slice (KILT/NQ/HotpotQA/FEVER depending on target),
- and load testing (latency/throughput under concurrency). citeturn11search0turn11search1turn6search0turn15search7

## Representative systems, tools, and products

### Comparison table

The table mixes *research systems* (architectures) with *production components* (databases/serving) because real external-memory LLM stacks are almost always composites.

| System / component | Memory type | Core idea | Retrieval / indexing | Update friendliness | Typical use |
|---|---|---|---|---|---|
| RAG (Lewis et al.) | Text passages (non-parametric) | Retrieve top‑k passages and condition generation on them | Dense vector index + neural retriever | High (swap/update corpus) | Knowledge-intensive QA, doc assistants citeturn0search0turn0search8 |
| REALM | Text corpus during pretraining | Learn retriever via LM objective; retrieval in loop | Latent retriever over large corpus | Medium–high (index updates possible; retriever learned) | Retrieval-aware pretraining citeturn0search1turn0search17 |
| Atlas | Text corpus + learned retriever | Jointly pretrain retriever + LM for few-shot | Retrieval-augmented LM | Medium–high | Few-shot knowledge tasks citeturn15search7turn15search15 |
| RETRO | Token chunk database | Retrieve similar chunks during generation; cross-attend | Large-scale retrieval + chunked cross-attention | Medium | Scaling LM performance with external data citeturn0search10turn0search2 |
| REPLUG | Retrieved docs + frozen LM | Improve black-box LMs via tunable retriever + prepending | Retriever tuned to LM | High | RAG with proprietary LMs citeturn6search2turn6search6 |
| Self‑RAG | Retrieved evidence + “reflection” | Retrieve on demand and critique using special tokens | Retrieval controller learned | Medium | Factual long-form generation citeturn6search7turn6search3 |
| GraphRAG | Graph + community summaries | Build graph over corpus; retrieve via graph structure | Graph construction + query-time augmentation | Medium (graph rebuild cadence) | Broad “global” questions over corpora citeturn15search0turn15search4 |
| HyDE (technique) | Dense retrieval improvement | Generate hypothetical doc, embed it, retrieve neighbours | Unsupervised dense retrieval pivot | High | Zero-shot retrieval boosts citeturn15search1turn15search13 |
| kNN‑LM | Activation datastore | Interpolate LM probs with kNN over hidden states | ANN over hidden-state keys | High (datastore swap) | Domain adaptation, rare facts citeturn4search1turn4search4 |
| Memorizing Transformers | Recent KV/activations | kNN lookup into memory of recent pairs | Approximate kNN lookup | Medium | Test-time memorisation within stream citeturn4search2turn4search5 |
| Infini-attention | Compressive internal memory | Bounded-memory streaming “infinite” context | Compressive memory in attention block | N/A (model-side) | Very long context inference citeturn8search1turn8search4 |
| Titans | Neural long-term memory module | “Learn to memorise at test time”; huge context claims | Memory module + attention | N/A (model-side) | Needle-in-haystack long contexts citeturn8search0turn8search3 |
| Letta (formerly MemGPT) | Tiered agent memory | Paging between context tiers; stateful agents | DB-backed memory + policies | High | Stateful assistants / agents citeturn16search0turn0search7 |
| A‑MEM | Structured notes + links | Zettelkasten-like dynamic note graph | Retrieval + linking + evolution | High | Long-lived agent memory organisation citeturn16search17turn16search1 |
| FAISS | Vector index library | Efficient similarity search; many index types | IVF, PQ, HNSW, GPU options | N/A (library) | Self-hosted ANN retrieval citeturn1search13turn1search10 |
| Milvus | Distributed vector DB | Separate compute/storage; distributed components | Multiple index types; GPU options | High | Large-scale vector retrieval citeturn14search4turn14search0turn1search5 |
| Pinecone | Managed vector DB | Pods/replicas scaling; managed ops | Managed ANN | High | Production managed RAG citeturn14search2turn1search2turn14search18 |
| Weaviate | Vector DB + hybrid | Vector + BM25 hybrid with fusion; replication | HNSW/flat; hybrid search | High | Hybrid semantic/keyword search citeturn1search6turn14search3turn1search16 |
| Qdrant | Vector DB with payload | Filterable ANN + shard replication | HNSW + payload indexes | High | Filter-heavy RAG and agents citeturn10search4turn14search1 |
| pgvector | Postgres extension | Store vectors with relational data | Exact + ANN indexes | High (ACID + SQL) | “RAG in Postgres” stacks citeturn10search2turn10search10 |
| Elasticsearch kNN | Search engine + vectors | Vector kNN integrated with search; segments/shards | HNSW approximate kNN | High | Hybrid search + enterprise ops citeturn10search3turn10search7turn10search11 |
| vLLM | Serving-time memory mgmt | PagedAttention for KV cache efficiency | KV cache paging/sharing | N/A (serving) | High-throughput LLM serving citeturn13search0turn13search12 |

### Notes on open-source vs commercial stacks

- Open-source RAG frameworks (LangChain, LlamaIndex, Haystack) mainly standardise *wiring*: loaders → chunkers → embedders → stores → retrievers → prompt engines; they don’t remove the need to engineer evaluation, freshness policies, and latency budgets. citeturn9search1turn9search2turn9search3  
- Managed vector DBs trade infra work (replication, monitoring, scaling) for cost and vendor constraints; Pinecone’s docs make scaling knobs explicit (pods vs replicas). citeturn14search2turn1search2  
- “RAG in your existing DB” is increasingly viable: pgvector explicitly targets storing vectors alongside relational data, making joins and ACL filtering straightforward. citeturn10search2turn10search10

## Trade-offs, best practices, and future directions

### Key trade-offs

**Retrieval vs longer context.** Longer context and better attention kernels reduce the need for retrieval, but they raise decoding costs and KV-cache pressure; retrieval keeps prompts short but risks missing context and adds index complexity. Transformer‑XL/Longformer/Infini-attention/Titans represent the “make context huge” line, while RAG/Atlas/RETRO represent “keep model smaller, fetch what you need.” citeturn5search3turn7search1turn8search1turn8search0turn0search0turn15search7turn0search10

**Quality vs latency in retrieval.** Late-interaction / re-ranking can improve retrieval quality (BEIR discusses this class), but they add compute; hybrid search can improve recall on keyword-heavy queries but adds tuning complexity. citeturn11search0turn1search6turn12search5

**Durability vs agility in memory writes.** Agentic memory systems that evolve graphs/links (A‑MEM, GraphRAG-like builds) can improve long-term coherence, but they introduce background jobs (link generation, summarisation, graph rebuilds) and more surface area for inconsistency. citeturn16search17turn15search0turn14search0

**Black-box LLM integration vs deep integration.** REPLUG and many production RAG stacks work with closed LLMs because they only need prompting; kNN-LM / Memorizing Transformers require internal representations and thus tighter model integration. citeturn6search2turn4search1turn4search2

### Best practices that consistently move the needle

1. **Treat retrieval as a first-class model with its own benchmarks.** Use BEIR-style retrieval evaluation to tune embedding models, chunking, and ANN parameters before blaming the generator. citeturn11search0turn9search6turn2search0  
2. **Use metadata aggressively.** Filtering (ACLs, tenant) and freshness are easier and safer as metadata constraints than as prompt instructions; vector DB docs emphasise payload-based filtering and replication/sharding semantics. citeturn10search4turn14search1turn14search3  
3. **Adopt multi-stage retrieval.** Cheap broad retrieval → stronger re-ranking → tight context packing routinely beats “top‑k and pray,” and it matches the cost-quality story in BEIR and many RAG recipes. citeturn11search0turn0search0  
4. **Make context assembly observable.** Store which chunks were retrieved, their scores, and which were actually injected; evaluation tools like RAGAS target these dimensions explicitly. citeturn11search2turn11search14  
5. **Engineer caches explicitly.** Use semantic caches for repeated questions (GPTCache) and KV-cache/prefix optimisations for throughput (vLLM), because long contexts make serving memory a dominant cost. citeturn13search2turn13search0

### Future directions (where research is heading)

- **Retrieval becomes adaptive and self-critical by default.** Self‑RAG is a clear signal: retrieval should be on-demand, and generation should be coupled to critique/citation verification. citeturn6search7turn6search3  
- **Memory policies become learned modules.** Titans proposes test-time memory modules; MemGen positions latent memory as part of cognition loops; these point toward “memory as computation,” not just storage. citeturn8search0turn8search7  
- **Graph + text hybrids for “global questions.”** GraphRAG-like approaches aim to answer questions that require global synthesis across large corpora, where naïve chunk retrieval struggles. citeturn15search0turn15search12  
- **Serving-time memory management remains a bottleneck.** As contexts grow, KV-cache and attention efficiency (FlashAttention, PagedAttention) become central to making any memory-augmented system economical. citeturn13search1turn13search0  
- **Operational semantics matter more: permissions, provenance, and update correctness.** As RAG is used on private corpora, correctness is not just “answer accuracy” but “answer uses authorised, current sources,” pushing metadata, auditing, and evaluation tooling into the core loop. citeturn10search4turn11search2turn15search4

### Selected primary links (papers + official docs)

To keep this actionable, here are direct URLs to foundational and cutting-edge sources cited throughout (papers first, then docs/tools):

```text
RAG (2020): https://arxiv.org/abs/2005.11401
REALM (2020): https://arxiv.org/abs/2002.08909
RETRO (2021): https://arxiv.org/abs/2112.04426
Atlas (2022/2023): https://arxiv.org/abs/2208.03299
REPLUG (2023/2024): https://arxiv.org/abs/2301.12652
Self-RAG (ICLR 2024): https://arxiv.org/abs/2310.11511
GraphRAG (2024): https://arxiv.org/abs/2404.16130
HyDE (2022/2023): https://arxiv.org/abs/2212.10496

kNN-LM (ICLR 2020): https://arxiv.org/abs/1911.00172
Memorizing Transformers (2022): https://arxiv.org/abs/2203.08913
Infini-attention (2024): https://arxiv.org/abs/2404.07143
Titans (2024): https://arxiv.org/abs/2501.00663
A-MEM (2025): https://arxiv.org/abs/2502.12110
MemGPT (2023): https://arxiv.org/abs/2310.08560
Generative Agents (2023): https://arxiv.org/abs/2304.03442

HNSW (paper): https://arxiv.org/abs/1603.09320
DiskANN (2019): https://www.microsoft.com/en-us/research/publication/diskann-fast-accurate-billion-point-nearest-neighbor-search-on-a-single-node/
ScaNN / anisotropic VQ (ICML 2020): https://proceedings.mlr.press/v119/guo20h/guo20h.pdf
BEIR (2021): https://arxiv.org/abs/2104.08663
KILT (2021): https://aclanthology.org/2021.naacl-main.200/
RAGAS (2023/2024): https://arxiv.org/abs/2309.15217

FAISS docs: https://faiss.ai/
FAISS index guide: https://github.com/facebookresearch/faiss/wiki/Faiss-indexes
Milvus docs: https://milvus.io/docs/overview.md
Qdrant docs: https://qdrant.tech/documentation/overview/
Weaviate docs: https://docs.weaviate.io/weaviate/
Pinecone docs: https://docs.pinecone.io/
pgvector (GitHub): https://github.com/pgvector/pgvector
Elasticsearch kNN docs: https://www.elastic.co/docs/solutions/search/vector/knn
vLLM / PagedAttention (paper): https://arxiv.org/abs/2309.06180
FlashAttention (paper): https://arxiv.org/abs/2205.14135
Letta (formerly MemGPT): https://github.com/letta-ai/letta
A-MEM code: https://github.com/agiresearch/A-mem
LlamaIndex docs: https://developers.llamaindex.ai/python/framework/
LangChain retrieval docs: https://docs.langchain.com/oss/python/langchain/retrieval
Haystack docs: https://docs.haystack.deepset.ai/
```

Primary/official sources for the tools above are also referenced throughout via citations. citeturn0search0turn0search1turn0search10turn15search7turn6search2turn6search7turn15search0turn15search1turn4search1turn4search2turn8search1turn8search0turn16search17turn0search7turn9search0turn2search0turn2search19turn3search0turn11search0turn11search1turn11search2turn1search13turn14search4turn14search5turn14search3turn1search2turn10search2turn10search3turn13search0turn13search1turn16search0turn16search1turn16search10turn9search1turn9search3