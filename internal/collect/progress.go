package collect

// Event is a best-effort presentation update. The report is authoritative;
// a slow or closed view must never block collection or evidence finalization.
// The caller owns the channel and must not close it while Run is active.
type Event struct{ Device, Command, Status string }

func emit(o Options, device, command, status string) {
	if o.Events == nil {
		return
	}
	select {
	case o.Events <- Event{Device: device, Command: command, Status: status}:
	default:
	}
}
