/*
	Copyright (c) 2013 Ondřej Kupka

	Permission is hereby granted, free of charge, to any person obtaining a copy of
	this software and associated documentation files (the "Software"), to deal in
	the Software without restriction, including without limitation the rights to
	use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
	the Software, and to permit persons to whom the Software is furnished to do so,
	subject to the following conditions:

	The above copyright notice and this permission notice shall be included in all
	copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
	IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
	FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
	COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
	IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
	CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package statemachine

import (
	"fmt"
	"testing"
)

// Examples -------------------------------------------------------------------

const (
	stateStopped State = iota
	stateRunning
	stateClosed
)

func stateToString(s State) string {
	return [...]string{
		stateStopped: "STOPPED",
		stateRunning: "RUNNING",
		stateClosed:  "CLOSED",
	}[s]
}

const (
	cmdRun EventType = iota
	cmdStop
	cmdClose
)

func cmdToString(t EventType) string {
	return [...]string{
		cmdRun:   "RUN",
		cmdStop:  "STOP",
		cmdClose: "CLOSE",
	}[t]
}

type Context struct {
	seq int
}

func (ctx *Context) handleRun(s State, e *Event) (next State) {
	ctx.seq += 1
	fmt.Printf("Event number %d received\n", ctx.seq)

	fmt.Printf("%s -> %s by %s\n", stateToString(s), stateToString(stateRunning), cmdToString(e.Type))
	return stateRunning
}

func (ctx *Context) handleStop(s State, e *Event) (next State) {
	ctx.seq += 1
	fmt.Printf("Event number %d received\n", ctx.seq)

	fmt.Printf("%s -> %s by %s\n", stateToString(s), stateToString(stateStopped), cmdToString(e.Type))
	return stateStopped
}

func (ctx *Context) handleClose(s State, e *Event) (next State) {
	ctx.seq += 1
	fmt.Printf("Event number %d received\n", ctx.seq)

	fmt.Printf("%s -> %s by %s\n", stateToString(s), stateToString(stateClosed), cmdToString(e.Type))
	return stateClosed
}

func ExampleStateMachine() {
	// Allocate space for 3 states, 3 commands and 10 requests in the channel.
	sm := New(stateStopped, 3, 3, 10)

	// Allocate a new Context which is going to keep our data between
	// the handler calls.
	ctx := new(Context)
	fmt.Printf("Context struct: %#v\n", ctx)

	// RUN
	sm.On(cmdRun, []State{
		stateStopped,
	}, ctx.handleRun)

	// STOP
	sm.On(cmdStop, []State{
		stateRunning,
	}, ctx.handleStop)

	// CLOSE
	sm.On(cmdClose, []State{
		stateStopped,
		stateRunning,
	}, ctx.handleClose)

	var (
		run  = &Event{cmdRun, nil}
		stop = &Event{cmdStop, nil}
		cls  = &Event{cmdClose, nil}
	)

	sm.Emit(run, nil)
	sm.Emit(stop, nil)
	sm.Emit(run, nil)
	sm.Emit(stop, nil)

	// Show how to write an event producer.
	exit := make(chan struct{})

	go func() {
		eventCh := make(chan *Event)
		go func() {
			eventCh <- run
			eventCh <- stop
			eventCh <- run
			eventCh <- cls
			close(exit)
		}()

		for {
			select {
			case event := <-eventCh:
				sm.Emit(event, nil)
			case <-sm.TerminatedChannel():
				return
			}
		}
	}()

	// Wait for the inner goroutine to close the exit channel.
	<-exit

	// Close our state machine.
	sm.Terminate()

	// Wait for the state machine to terminate.
	<-sm.TerminatedChannel()

	fmt.Println("Goroutine exited")
	// Output:
	// Context struct: &statemachine.Context{seq:0}
	// Event number 1 received
	// STOPPED -> RUNNING by RUN
	// Event number 2 received
	// RUNNING -> STOPPED by STOP
	// Event number 3 received
	// STOPPED -> RUNNING by RUN
	// Event number 4 received
	// RUNNING -> STOPPED by STOP
	// Event number 5 received
	// STOPPED -> RUNNING by RUN
	// Event number 6 received
	// RUNNING -> STOPPED by STOP
	// Event number 7 received
	// STOPPED -> RUNNING by RUN
	// Event number 8 received
	// RUNNING -> CLOSED by CLOSE
	// Goroutine exited
}

// Tests ----------------------------------------------------------------------

func TestStateMachine_ReturnErrIllegalEvent(test *testing.T) {
	sm := New(stateStopped, 3, 3, 0)
	defer sm.Terminate()

	errCh := make(chan error, 1)
	sm.Emit(&Event{cmdStop, nil}, errCh)
	if err := <-errCh; err != ErrIllegalEvent {
		test.Errorf("Unexpected error received: %s", err)
	}
}

func TestStateMachine_ReturnErrTerminated(test *testing.T) {
	sm := New(stateStopped, 3, 3, 0)
	err := sm.Terminate()
	if err != nil {
		test.Fatal(err)
	}
	if err := sm.Terminate(); err != ErrTerminated {
		test.Errorf("Unexpected error received: %s", err)
	}
}

// Benchmarks -----------------------------------------------------------------

func BenchmarkStateMachine(bm *testing.B) {
	sm := New(stateStopped, 3, 3, 0)
	sm.On(cmdStop, []State{
		stateStopped,
	}, func(s State, e *Event) State {
		return s
	})

	bm.ResetTimer()

	for i := 0; i < bm.N; i++ {
		exit := make(chan error, 1)
		sm.Emit(&Event{cmdStop, nil}, exit)
		<-exit
	}
}
