package main

import (
	"time"
)

// Debounces events received on a channel, returning the last event received
// after the delay is past.
func debounce[T interface{}](delay time.Duration, events chan T) chan T {
	return debounceWithBuffer(delay, events, 0)
}

// Debounces events received on a channel, returning the last event received
// after the delay is past.
func debounceWithBuffer[T interface{}](delay time.Duration, events chan T, outputBufferSize int) chan T {
	output := make(chan T, outputBufferSize)

	go func() {
		for event := range events {
		L:
			for {
				select {
				case tempEvent, ok := <-events:
					if !ok {
						// But if events is closed go ahead and output the last event and break out of the inner loop
						// which will then fall out of the outer loop and close the output
						output <- event
						break L
					} else {
						// Replaces the `event` local with the new event that was received while waiting
						event = tempEvent
					}

				case <-time.After(delay):
					// Forward the final event present after the delay and break out of the inner loop
					// which will wait for the next event to arrive
					output <- event
					break L
				}
			}
		}

		// Close the output once the input is closed
		close(output)
	}()

	return output
}
