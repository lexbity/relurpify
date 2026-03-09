## SkillSpec v2

`SkillSpec v2` extends skills from prompt snippets plus fixed capability lists into a
shared policy layer that can guide multiple agent implementations without
bypassing registry permissions, agent allowlists, or gVisor sandboxing.

### Key additions

- `phase_capability_selectors`
  - ordered capability selectors per phase
  - supports exact capability names and tag-based selection
- `verification.success_capability_selectors`
  - resolves success capabilities from exact names or capability tags
- `recovery.failure_probe_capability_selectors`
  - resolves ordered recovery probes from exact names or capability tags
- `planning`
  - required discovery/probe steps before editing
  - preferred edit and verify capability families
  - reusable step templates
- `review`
  - review criteria
  - approval rules
  - severity weighting hints

### Capability Selector Shape

```yaml
capability: "go_test"
tags: ["lang:go", "test"]
exclude_tags: ["destructive"]
```

Resolution rules:

1. selectors never grant access
2. resolution only considers capabilities already registered and allowed
3. explicit capability names win over tag matches
4. tag selectors require all listed tags and reject excluded tags

### Runtime flow

1. `ApplySkills` merges raw `SkillSpec v2` data into `AgentSkillConfig`
2. `skills.ResolveSkillPolicy` resolves selectors against the current capability registry
3. agents consume the resolved policy for:
   - phase disclosure
   - verification success matching
   - recovery probe ordering
   - planning hints
   - review hints

### Security model

Skills remain policy hints only. They cannot:

- register capabilities
- widen `allowed_capabilities`
- bypass permission manager checks
- bypass gVisor executable or filesystem restrictions

They can only narrow or prioritize behavior inside the existing security
envelope.
