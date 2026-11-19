// Package ports declares the interfaces consumed by the TUI and implemented by adapters.
//
// Rules:
//   - every method takes a context.Context and returns as soon as it is cancelled;
//   - listings are bounded by the domain limits or paginated, never unbounded;
//   - the adapter owning an Events channel closes it on cancellation, never the caller;
//   - the Git adapter serialises writes so concurrent calls cannot race the index lock.
package ports
