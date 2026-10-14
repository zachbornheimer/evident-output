package project

// CancellationPartialChangesNote is the one line a cancelled run adds when
// effects were already committed. It states that changes stand (cancellation
// never implies rollback, CONTRACT §8) without repeating the [changed]
// ledger that already names them.
const CancellationPartialChangesNote = "partial changes were applied before cancellation"
