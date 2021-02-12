// Info: The idea about the ring buffer is that we have a FIFO
// buffer where we store all incomming messages requested by
// operators. Each message processed will also be stored in a DB.
//
// Idea: All incomming messages should be handled from the in-memory
// buffered channel, but when they are put on the buffer they should
// also be written to the DB with a handled flag set to false.
// When a message have left the buffer the handled flag should be
// set to true.
package steward

import "fmt"

// ringBuffer holds the data of the buffer,
type ringBuffer struct {
	buf chan subjectAndMessage
}

// newringBuffer is a push/pop storage for values.
func newringBuffer(size int) *ringBuffer {
	return &ringBuffer{
		buf: make(chan subjectAndMessage, size),
	}
}

// start will process incomming messages through the inCh,
// and deliver messages out when requested on the outCh.
func (s *ringBuffer) start(inCh chan subjectAndMessage, outCh chan subjectAndMessage) {
	// Starting both writing and reading in separate go routines so we
	// can write and read concurrently.

	// Fill the buffer when new data arrives
	go func() {
		for v := range inCh {
			s.buf <- v
			fmt.Printf("**BUFFER** DEBUG PUSHED ON BUFFER: value = %v\n\n", v)
		}
		close(s.buf)
	}()

	// Empty the buffer when data asked for
	go func() {
		for v := range s.buf {
			outCh <- v
		}

		close(outCh)
	}()
}
