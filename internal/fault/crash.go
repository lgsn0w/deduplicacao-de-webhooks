package fault

import "os"

// CrashPoint identifies where in the processing pipeline a simulated crash occurs.
type CrashPoint string

const (
	CrashNone         CrashPoint = ""         // no crash — normal operation
	CrashAfterDBWrite CrashPoint = "db_write" // crash after DB commit, before publish
	CrashAfterPublish CrashPoint = "publish"  // crash after publish, before ACK
)

// Hook triggers a simulated process exit at a configured crash point.
type Hook struct {
	point  CrashPoint
	exitFn func(int)
}

// NewHook creates a Hook from the CRASH_AFTER environment variable.
// If unset or empty, the hook never fires.
func NewHook() *Hook {
	return &Hook{
		point:  CrashPoint(os.Getenv("CRASH_AFTER")),
		exitFn: os.Exit,
	}
}

// NewHookWithExit creates a Hook with an injectable exit function (for tests).
func NewHookWithExit(point CrashPoint, exitFn func(int)) *Hook {
	return &Hook{
		point:  point,
		exitFn: exitFn,
	}
}

// CheckAndCrash calls exitFn(1) if at matches the configured crash point.
func (h *Hook) CheckAndCrash(at CrashPoint) {
	if h.point == at {
		h.exitFn(1)
	}
}

// Point returns the configured crash point (for startup logging).
func (h *Hook) Point() CrashPoint {
	return h.point
}
