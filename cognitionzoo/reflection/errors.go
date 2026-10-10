package reflection

import "errors"

// ErrInvalidReview marks a review phase whose model output could not be parsed
// (a verdict outside pass|fail, or malformed JSON). It is classified as
// model_invalid_output by the node boundary, whose protocol retries the phase
// once before applying the step's on_error policy. A `fail` verdict is NOT an
// error — it is the quality signal the loop routes on.
var ErrInvalidReview = errors.New("reflection: invalid review output")
