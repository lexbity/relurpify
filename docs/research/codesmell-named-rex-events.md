# Codesmell: `named/rex/events`

Observed while expanding test coverage in canonical event normalization.

## Notes

- `events.go` combines trust normalization, payload coercion, envelope mapping, task mapping, and helper conversions in one file.
- `ToEnvelope` and `ToTask` duplicate a fair amount of payload extraction and metadata copying logic.
- `taskTypeForEvent`, `timeValue`, `stringSlice`, and the other helper converters are simple individually, but together they indicate a dense normalization layer with many edge-case branches.

## Risk

- Maintainability risk: the file is doing transport normalization and task construction at the same time, so changes to event shape or ingress rules can ripple across multiple helpers.
- Testability risk: many helpers are branchy coercion functions, so small payload-shape changes can affect several code paths at once.

## Tradeoff

- This concentration is understandable because rex needs a single canonical event boundary that can accept different ingress shapes and still produce consistent tasks and envelopes.

