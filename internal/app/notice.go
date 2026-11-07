package app

// Notice is a dictation failure the user can act on. Its message is short
// enough to show directly in the menu bar, unlike ordinary errors, which are
// summarised there and detailed in the diagnostic log.
type Notice struct {
	Message string
}

// NoSpeechMessage is shown when a recording transcribes to nothing.
const NoSpeechMessage = "No speech recognised"

func (n *Notice) Error() string {
	return n.Message
}
