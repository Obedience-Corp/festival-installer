package app

// ProgressEvent reports a stage of an install/update operation for TUI chrome.
type ProgressEvent struct {
	Stage   string // resolve | download | verify | extract | activate | done
	Package string
	Percent float64 // 0..1, or -1 when unknown
	Bytes   int64
	Message string
}

// ProgressFunc is called with stage updates; may be nil.
type ProgressFunc func(ProgressEvent)

func report(p ProgressFunc, ev ProgressEvent) {
	if p != nil {
		p(ev)
	}
}
