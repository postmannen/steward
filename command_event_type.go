// NB:
// When adding new constants for the Method or CommandOrEvent
// types, make sure to also add them to the map
// <Method/CommandOrEvent>Available since the this will be used
// to check if the message values are valid later on.

package steward

// Event describes on the message level if this is
// an event or command kind of message in the Subject name.
// This field is mainly used to be able to spawn up different
// worker processes based on the Subject name so we can have
// one process for handling event kind, and another for
// handling command kind of messages.
// This type is used in both building the subject name, and
// also inside the Message type to describe if it is a Command
// or Event.
type Event string

func (c Event) CheckEventAvailable() EventAvailable {
	ma := EventAvailable{
		topics: map[Event]struct{}{
			EventACK:  {},
			EventNACK: {},
		},
	}

	return ma
}

const (
	// EventACK, wait for the return of an ACK message.
	// The sender will wait for an ACK reply message
	// to decide if it was succesfully delivered or not.
	// If no ACK was received within the timeout, the
	// message will be resent the nr. of times specified
	// in retries field of the message.
	EventACK Event = "EventACK"
	// Same as above, but No ACK.
	EventNACK Event = "EventNACK"
)

// CommandOrEventAvailable are used for checking if the
// commands or events are defined.
type EventAvailable struct {
	topics map[Event]struct{}
}

// Check if a command or even exists.
func (e EventAvailable) CheckIfExists(event Event, subject Subject) bool {
	_, ok := e.topics[event]
	if ok {
		// log.Printf("info: CommandOrEventAvailable.CheckIfExists: command or event found: %v, for %v\n", c, subject.name())
		return true
	} else {
		// log.Printf("error: CommandOrEventAvailable.CheckIfExists: command or event not found: %v, for %v\n", c, subject.name())
		return false
	}
}
