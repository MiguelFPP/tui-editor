package fakes

import "sync"

// Call is one recorded invocation of a fake port method.
type Call struct {
	Method string
	Args   []any
}

type recorder struct {
	mu    sync.Mutex
	calls []Call
}

func (r *recorder) record(method string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, Call{Method: method, Args: args})
}

// Calls returns a copy of the recorded calls.
func (r *recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Call(nil), r.calls...)
}

func (r *recorder) CallCount(method string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, call := range r.calls {
		if call.Method == method {
			count++
		}
	}

	return count
}

func (r *recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = nil
}

// hook reads a hook under the recorder lock so concurrent calls stay race-free.
func hook[F any](r *recorder, fn *F) F {
	r.mu.Lock()
	defer r.mu.Unlock()

	return *fn
}
