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
	"os"
	"testing"
	"time"
)

// Examples -------------------------------------------------------------------

const (
	stateStopped State = iota
	stateRunning
	stateClosed
)

var (
	run  = &Event{cmdRun, nil}
	stop = &Event{cmdStop, nil}
	cls  = &Event{cmdClose, nil}
	sm   *StateMachine
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
	fmt.Fprintln(os.Stderr, "before emit")
	sm.Emit(stop)
	fmt.Fprintln(os.Stderr, "after emit")
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
	sm = New(stateStopped, 3, 3)

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

	sm.Emit(run)
	sm.Emit(stop)
	sm.Emit(run)
	sm.Emit(stop)

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
				sm.Emit(event)
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

func forwardState(s State, e *Event) State {
	return s
}

func TestStateMachine_On(t *testing.T) {
	sm := New(stateStopped, 3, 3)
	defer sm.Terminate()

	sm.On(cmdRun, []State{
		stateStopped,
	}, forwardState)

	ok, err := sm.IsHandlerAssigned(cmdRun, stateStopped)
	if err != nil {
		t.Error(err)
	}
	if !ok {
		t.Fail()
	}
}

func TestStateMachine_Off(t *testing.T) {
	sm := New(stateStopped, 3, 3)
	defer sm.Terminate()

	sm.On(cmdRun, []State{
		stateStopped,
	}, forwardState)

	sm.Off(cmdRun, stateStopped)

	ok, err := sm.IsHandlerAssigned(cmdRun, stateStopped)
	if err != nil {
		t.Error(err)
	}
	if ok {
		t.Fail()
	}
}

func TestStateMachine_IsHandlerAssigned(t *testing.T) {
	sm := New(stateStopped, 3, 3)
	defer sm.Terminate()

	sm.On(cmdRun, []State{
		stateStopped,
	}, forwardState)

	ok, err := sm.IsHandlerAssigned(cmdRun, stateStopped)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fail()
	}

	ok, err = sm.IsHandlerAssigned(cmdStop, stateRunning)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fail()
	}
}

func TestStateMachine_Emit(t *testing.T) {
	sm := New(stateStopped, 3, 3)
	defer sm.Terminate()

	emitCh := make(chan struct{})

	sm.On(cmdRun, []State{
		stateStopped,
	}, func(s State, e *Event) State {
		ch := e.Data.(chan struct{})
		close(ch)
		return s
	})

	if err := sm.Emit(&Event{
		cmdRun,
		emitCh,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-emitCh:
		break
	case <-time.After(time.Second):
		t.Error("Test timed out")
	}
}

func TestStateMachine_SetState(t *testing.T) {
	// Start in STOPPED.
	sm := New(stateStopped, 3, 3)
	defer sm.Terminate()

	// Allow RUNNING -> STOPPED.
	sm.On(cmdStop, []State{
		stateRunning,
	}, forwardState)

	// Set state to RUNNING.
	sm.SetState(stateRunning)

	// Emit STOP, which should pass if we are in RUNNING.
	if err := sm.Emit(&Event{cmdStop, nil}); err != nil {
		t.Fatal(err)
	}
}

func TestStateMachine_Terminate(t *testing.T) {
	sm := New(stateStopped, 3, 3)

	err := sm.Terminate()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-sm.TerminatedChannel():
		break
	case <-time.After(time.Second):
		t.Error("Test timed out")
	}
}

func TestStateMachine_FailWithErrIllegalEvent(t *testing.T) {
	sm := New(stateStopped, 3, 3)
	defer sm.Terminate()

	if err := sm.Emit(&Event{cmdStop, nil}); err != ErrIllegalEvent {
		t.Errorf("Unexpected error received: %s", err)
	}
}

func TestStateMachine_FailWithErrTerminated(t *testing.T) {
	sm := New(stateStopped, 3, 3)
	err := sm.Terminate()
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Terminate(); err != ErrTerminated {
		t.Errorf("Unexpected error received: %s", err)
	}
}

// Benchmarks -----------------------------------------------------------------

func BenchmarkStateMachine(b *testing.B) {
	sm := New(stateStopped, 3, 3)
	sm.On(cmdStop, []State{
		stateStopped,
	}, forwardState)

	b.ResetTimer()

	var err error
	for i := 0; i < b.N; i++ {
		if err = sm.Emit(&Event{cmdStop, nil}); err != nil {
			b.Fatal(err)
		}
	}
}
