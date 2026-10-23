# Chapter 1 — baseline transcripts (captured)

Every block below is real output from a real run on the verification
machine: Linux 6.18, Go 1.27.3, Ollama 0.40.8 serving `gemma4:e4b`,
Docker 29.9.2, `runsc` release-20260406.0, Python 3.14.8, pytest 9.1.1.
The tree the binaries were built from reports
`service.version v0.0.0-20261020195847-1bc57c620296+dirty` in its own
telemetry. Scratch paths are normalized to `~/code/stringkit` in the
chapter; elisions are marked.

## 1. Build and vet

```text
$ go build ./...        # no output; exit 0
$ go vet ./...          # no output; exit 0
```

(`time` reported 1.4s wall on a warm cache; a first build takes a minute
or two.)

## 2. The baseline gate run

```text
$ make lint-arch
[PASS] lint-class-normalization: all class casts normalize
[PASS] lint-no-permissive-hitl: no permissive HITL broker
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/envcheck
[PASS] envcheck: no env access outside userconfig (1087 files scanned)
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/shimcheck
[PASS] shimcheck: no forbidden language (1087 files scanned)
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/symcheck
[PASS] symcheck: no removed symbols (1087 files scanned)
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/archcheck; EXIT_CODE=$?; \
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=direction; \
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=governance-orch; \
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=context-ports; \
exit $EXIT_CODE
[PASS] forbidden: no violations
[PASS] converter: no violations
[PASS] sqlite: no violations
[PASS] glob: no violations
[PASS] stub: no violations
[PASS] cycle: no violations
[PASS] bucket: no violations
[PASS] consumer: no violations
[PASS] All architecture gates passed
[PASS] domain-direction: no violations
[PASS] All domain checks passed
[PASS] governance-no-orchestration: no violations
[PASS] All domain checks passed
[PASS] context-ports: no violations
[PASS] All domain checks passed
```

(3.1s wall, warm cache.)

## 3. stringkit reset — tests green

```text
$ python3 -m pytest test_stringkit.py -q
..                                                                       [100%]
2 passed in 0.02s
```

## 4. Doctor on the stringkit workspace (first run; tail of report)

```text
Result: ready
```

Full report matches book 1's Chapter 1 shape: providers probed, `runner:
missing [warning]` and `runtime_image: degraded [warning]` informational,
verdict `Result: ready`. The materialized profile catalog in this tree is
`default.llm.yaml`, `gemma4.llm.yaml`, `qwen2.5-coder.llm.yaml`; the
workspace runs `model.name: gemma4:e4b`.

## 5. One stringkit turn — the arc, exactly as it happened

### Turn 1 — with the materialized profile (`tool_calling.intent: auto`)

The model's tool decision arrived as raw text and the run closed without
executing anything:

```text
│ 🤖 [10:05] Agent                                                   │
│ 🤔 Thinking [−]                                                    │
│ └─ 🔍 Analyzing request (4s)                                       │
│                                                                    │
│ {"thought":"I need to find and read the content of 'stringkit.py'  │
│ to explain the 'slugify' function. I will start by listing the     │
│ files to locate the file.","action":"file_list","tool":"file_list", │
│ "arguments":{"directory":"."}}<tool_call|><|tool_response>          │
│                                                                    │
│ 🧾 Result                                                          │
│ node=euclo.done | status=ok                                        │
│ ⏱  4s | 29 tok                                                     │
```

(Panels reflowed from the 120-column capture.)

### The fix — one line in a file you own

```text
$ sed -i 's/  intent: auto/  intent: prompt_based/' \
  relurpify_cfg/model/profiles/default.llm.yaml
```

### Turn 2 — after the fix, TUI restarted

The model produced a real explanation (abridged here; full JSON in the
chapter), and the run was flagged:

```text
│ 🧾 Result                                                          │
│ node=euclo.done | status=ok                                        │
│ ⏱  2s | 29 tok                                                     │
╰────────────────────────────────────────────────────────────────────╯
│ ⚙ [10:08] System                                                   │
│ Agent error: task edits code but the run executed no file write or │
│ edit observation                                                   │
```

The flag is `cognitionzoo/react/react_observe_node.go` loop-close honesty:
"an edit-shaped task that closed without an executed edit observation is
unverified no matter who claimed completion — the model asserting success
is not evidence." At this tree, chat-routed react runs are edit-shaped
regardless of phrasing (tried: "explain …", "analyze … and explain …",
"explain … without modifying anything", "investigate …" — all flagged),
so read-only chat turns close unverified.
`[NEEDS CONFIRMATION: the exact chat→react task-type wiring at
BETA-RC1-FALL2026]`

## 6. The receipts

Audit chain (`~/.relurpify_state/audit/agent-stringkit-euclo/chain-*.jsonl`,
trimmed to the decision fields):

```json
{"action": "tool:file_list", "type": "hitl", "result": "tool_allowed", "timestamp": "2026-10-23T17:05:06.99Z"}
{"action": "tool:file_read", "type": "hitl", "result": "tool_allowed", "timestamp": "2026-10-23T17:06:11.79Z"}
```

Telemetry spans for one turn (`.relurpify_state/telemetry/otlp/*.jsonl`;
line records carry bodies, decisions ride on spans):

```text
node euclo.intake
node euclo.family_select
node euclo.ingest
node euclo.stream
node euclo.checkpoint
node euclo.interaction_check
node euclo.policy_gate
node euclo.dispatch
node euclo.route_fork
node euclo.execute_thoughtrecipe
node euclo.merge
node euclo.report
node react_think / react_act / react_observe / react_done   (per react run)
```

Insertion decisions recorded, e.g.:

```text
insertion decision recorded  run.id=b9ccf2485d13adf811030053 agent.id=agent-stringkit-euclo
  node.id=react_act capability=file_list capability_id=tool:file_list
  insertion=direct security_event=insertion_decision
```
