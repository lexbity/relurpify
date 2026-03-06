## SkillSpec v2

`SkillSpec v2` extends skills from prompt snippets plus fixed tool lists into a
shared policy layer that can guide multiple agent implementations without
bypassing registry permissions, agent allowlists, or gVisor sandboxing.

### Key additions

- `phase_selectors`
  - ordered tool selectors per phase
  - supports exact tool names and tag-based selection
- `verification.success_selectors`
  - resolves success tools from exact names or capability tags
- `recovery.failure_probe_selectors`
  - resolves ordered recovery probes from exact names or capability tags
- `planning`
  - required discovery/probe steps before editing
  - preferred edit and verify tool families
  - reusable step templates
- `review`
  - review criteria
  - approval rules
  - severity weighting hints

### Tool selector shape

```yaml
tool: "go_test"
tags: ["lang:go", "test"]
exclude_tags: ["destructive"]
```

Resolution rules:

1. selectors never grant access
2. resolution only considers tools already registered and allowed
3. explicit tool names win over tag matches
4. tag selectors require all listed tags and reject excluded tags

### Runtime flow

1. `ApplySkills` merges raw `SkillSpec v2` data into `AgentSkillConfig`
2. `toolsys.ResolveSkillPolicy` resolves selectors against the current tool registry
3. agents consume the resolved policy for:
   - phase disclosure
   - verification success matching
   - recovery probe ordering
   - planning hints
   - review hints

### Security model

Skills remain policy hints only. They cannot:

- register tools
- widen `allowed_tools`
- bypass permission manager checks
- bypass gVisor executable or filesystem restrictions

They can only narrow or prioritize behavior inside the existing security
envelope.
