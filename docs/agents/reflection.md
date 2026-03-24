# Reflection Agent

## Synopsis

Reflection is a review-and-revise runtime. It delegates the main task to
another executor, asks a reviewer model to critique the result, and decides
whether another iteration is needed.

It is effectively a quality-control wrapper around another runtime. Instead of
changing how the underlying task is executed, Reflection adds a structured
second opinion and a bounded revision loop on top of it.

## How It Works

1. A delegate agent performs the task and returns a result.
2. A reviewer model evaluates that result against the task and runtime
   guidance.
3. The runtime parses the review into a structured payload containing issues
   and an approval signal.
4. A decision node scores the review and determines whether revision is
   required.
5. If revision is required and the iteration budget is not exhausted, the
   delegate runs again.

Reflection is useful when output quality, reviewability, or safety matters more
than single-pass speed.

This is particularly valuable for tasks where the cost of a missed issue is
higher than the cost of one or two extra review passes. Reflection is a way to
spend more inference budget on checking and revision without redesigning the
delegate runtime itself.

## Runtime Behaviour

- Reflection requires both a delegate executor and a reviewer model; it will
  fail fast if either is missing.
- The runtime is implemented as a small graph with execute, review, decision,
  and done nodes.
- Delegate execution happens against a cloned child context so failed runs do
  not partially mutate the parent state.
- Review results are stored under `reflection.review`, with decisions and
  assessment data stored under keys such as `reflection.revise`,
  `reflection.iteration`, and `reflection.assessment`.
- The maximum number of reflection cycles defaults to three unless overridden
  through config.
- In the default named wiring, Reflection commonly wraps a `ReActAgent`, but it
  can review any compatible `graph.WorkflowExecutor`.

In practical use, Reflection is most effective when the delegate and reviewer
have distinct roles. The delegate focuses on completing the task; the reviewer
focuses on issue finding, verification expectations, and approval thresholds.
That separation makes the revision decision more explicit than simply asking a
single model to "double check itself" inside one prompt.
