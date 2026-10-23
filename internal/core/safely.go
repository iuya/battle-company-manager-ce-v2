package core

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// safely runs f and turns a panic into an error (logged with its stack): a goroutine outside the HTTP recover middleware would
// otherwise end the whole process.
func safely(what string, f func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("panic recovered", "in", what, "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("%s panicked: %v", what, p)
		}
	}()
	return f()
}
