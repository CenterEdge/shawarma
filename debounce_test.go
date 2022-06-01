package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDebounce_InputClosed_OutputClosed(t *testing.T) {
	assert := assert.New(t)

	input := make(chan struct{})

	output := debounce(50*time.Millisecond, input)

	close(input)

	_, ok := <-output

	assert.False(ok)
}

func TestDebounce_SingleEvent_Received(t *testing.T) {
	assert := assert.New(t)

	input := make(chan int)

	output := debounce(50*time.Millisecond, input)

	input <- 1
	received := <-output

	assert.Equal(1, received)

	close(input)
}

func TestDebounce_MultipleEvents_ReceiveLast(t *testing.T) {
	assert := assert.New(t)

	input := make(chan int)

	output := debounce(50*time.Millisecond, input)

	input <- 1
	input <- 2
	input <- 3
	received := <-output

	assert.Equal(3, received)

	close(input)
}

func TestDebounce_CloseAfterMultipleEvents_ReceiveLast(t *testing.T) {
	assert := assert.New(t)

	input := make(chan int)

	output := debounce(50*time.Millisecond, input)

	input <- 1
	input <- 2
	input <- 3
	close(input)
	received := <-output

	assert.Equal(3, received)
}

func TestDebounce_EventsAfterDelay_ReceivesBeforeAndAfter(t *testing.T) {
	assert := assert.New(t)

	input := make(chan int)

	output := debounce(10*time.Millisecond, input)

	start := time.Now()
	input <- 1
	input <- 2
	input <- 3
	received := <-output

	assert.Equal(3, received)
	assert.LessOrEqual(10*time.Millisecond, time.Since(start))

	start = time.Now()
	input <- 4
	input <- 5
	received = <-output

	assert.Equal(5, received)
	assert.LessOrEqual(10*time.Millisecond, time.Since(start))

	close(input)
}
