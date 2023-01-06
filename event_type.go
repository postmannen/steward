// NB:
// When adding new constants for the Method or event
// types, make sure to also add them to the map
// <Method/Event>Available since the this will be used
// to check if the message values are valid later on.

package steward

// Event describes on the message level if this is
// an ACK or NACK kind of message in the Subject name.
// This field is mainly used to be able to spawn up different
// worker processes based on the Subject name.
// This type is used in both building the subject name, and
// also inside the Message type to describe what kind like
// ACK or NACK it is.
type Event string

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
