// Package order owns orders, the order state machine, disputes and reviews.
//
// Boundary: owns `order`, `dispute`, `review`. ALL status transitions go
// through one state-machine function — no scattered `UPDATE status` anywhere.
// Every transition writes an audit_log entry.
//
// State machine (see docs §5.4):
//   created -> paid -> delivered -> confirmed -> settled        [normal terminal]
//   any active -> disputed -> refunded                          [refund terminal]
//   created -> cancelled (payment timeout)                      [terminal]
//
// Implemented in: PR-11 (orders + state machine), PR-18 (disputes/reviews).
package order
