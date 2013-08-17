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
	"errors"
)

// PUBLIC TYPES ---------------------------------------------------------------

type (
	State     int
	EventType int
	Context   interface{}
)

// Events are the basic units that can be processed by a state machine.
type Event struct {
	Type EventType
	Data interface{}
}

// Various EventHandlers can be registered to process events in particular states.
// By registering event handlers we build up a mapping of state x event -> handler
// and the handler is invoked exactly in the defined state when the defined event
// is emitted.
//
// Once a handler is invoked, its role is to take the StateMachine into the next
// state, doing some useful work on the way. There is a context variable that
// is to be used to keep come useful data between handler calls. So, the state
// is for consistency, the context is for keeping data.
//
// If an event is emitted in a state where no handler is defined,
// ErrIllegalEvent is returned.
type EventHandler func(state State, ctx Context, evt *Event) (next State)

// StateMachine is the only struct this package exports. Once an event is
// emitted on a StateMachine, the relevant handler is fetched and invoked.
// StateMachine takes care of all the synchronization, it is thread-safe.
// It does not use any locking, just channels. While that may be a bit more
// overhead, it is more robust and clear.
type StateMachine struct {
	state State
	ctx   Context

	handlers [][]EventHandler

	cmdCh        chan *command // Send commands to the background loop
	terminatedCh chan struct{} // Signal that the state machine is terminated
}

// CONSTRUCTOR ----------------------------------------------------------------

// Create new StateMachine. Allocate internal memory for particular number of
// states and events, set internal channel size. As long as the internal channel
// is not full, most of the methods are non-blocking.
func NewStateMachine(initState State, initCtx Context, stateCount, eventCount, mailboxSize uint) *StateMachine {
	// Allocate enough space for the handlers.
	table := make([][]EventHandler, stateCount)
	for i := range table {
		table[i] = make([]EventHandler, eventCount)
	}

	sm := StateMachine{
		state:        initState,
		ctx:          initCtx,
		handlers:     table,
		cmdCh:        make(chan *command, mailboxSize),
		terminatedCh: make(chan struct{}),
	}

	// Start background goroutine.
	go sm.loop()

	return &sm
}

// COMMANDS -------------------------------------------------------------------

const (
	cmdOn EventType = iota
	cmdIsHandlerDefined
	cmdOff
	cmdEmit
	cmdTerminate
)

type command struct {
	cmd  EventType
	args interface{}
}

// On -------------------------------------------------------------------------

type cmdOnArgs struct {
	s State
	t EventType
	h EventHandler
}

// Register an event handler. Only one handler can be set per state and event.
// It is non-blocking as long as the internal channel is not full.
func (sm *StateMachine) On(t EventType, s State, h EventHandler) error {
	return sm.send(&command{
		cmdOn,
		&cmdOnArgs{s, t, h},
	})
}

// IsHandlerDefined -----------------------------------------------------------

type cmdIsHandlerDefinedArgs struct {
	s  State
	t  EventType
	ch chan bool
}

// Check if a handler is defined for this state and event.
// It is non-blocking as long as the internal channel is not full.
func (sm *StateMachine) IsHandlerDefined(t EventType, s State) (defined bool, err error) {
	replyCh := make(chan bool, 1)
	err = sm.send(&command{
		cmdIsHandlerDefined,
		&cmdIsHandlerDefinedArgs{s, t, replyCh},
	})
	if err != nil {
		return
	}
	defined = <-replyCh
	return
}

// Off ------------------------------------------------------------------------

type cmdOffArgs struct {
	s State
	t EventType
}

// Drop a handler assigned to the state and event.
// It is non-blocking as long as the internal channel is not full.
func (sm *StateMachine) Off(t EventType, s State) error {
	return sm.send(&command{
		cmdOff,
		&cmdOffArgs{s, t},
	})
}

// Emit -----------------------------------------------------------------------

type cmdEmitArgs struct {
	e  *Event
	ch chan error
}

// Emit a new event. It is possible to pass a channel to the internal loop
// to check if the handler was found and scheduled for execution.
// It is non-blocking as long as the internal channel is not full.
func (sm *StateMachine) Emit(event *Event, replyCh chan error) error {
	return sm.send(&command{
		cmdEmit,
		&cmdEmitArgs{event, replyCh},
	})
}

// Terminate ------------------------------------------------------------------

// Terminate the internal event loop and close all internal channels.
// Particularly the termination channel is closed to signal all producers that
// they can no longer emit any events and shall exit.
// It is non-blocking as long as the internal channel is not full.
func (sm *StateMachine) Terminate() error {
	return sm.send(&command{
		cmdTerminate,
		nil,
	})
}

// TerminateChannel can be used to obtain a channel that is closed once
// the state machine is terminated and is no longer willing to accept any events.
// This is useful if you want to start multiple goroutines to asynchronously
// post events. You can just start them, pass them this termination channel
// and leave them be. The only requirement is that those producer goroutines
// should exit or simply stop posting any events as soon as the channel is closed.
func (sm *StateMachine) TerminatedChannel() chan struct{} {
	return sm.terminatedCh
}

// INTERNALS ------------------------------------------------------------------

// Helper method for sending events to the internal event loop.
func (sm *StateMachine) send(cmd *command) error {
	select {
	case sm.cmdCh <- cmd:
		return nil
	case <-sm.terminatedCh:
		return ErrTerminated
	}
}

// The internal event loop processes events (commands) passed to it in
// a sequential manner.
func (sm *StateMachine) loop() {
	for {
		cmd := <-sm.cmdCh
		switch cmd.cmd {
		case cmdOn:
			args := cmd.args.(*cmdOnArgs)
			sm.handlers[args.s][args.t] = args.h
		case cmdIsHandlerDefined:
			args := cmd.args.(*cmdIsHandlerDefinedArgs)
			args.ch <- (sm.handlers[args.s][args.t] == nil)
			close(args.ch)
		case cmdOff:
			args := cmd.args.(*cmdOffArgs)
			sm.handlers[args.s][args.t] = nil
		case cmdEmit:
			args := cmd.args.(*cmdEmitArgs)
			handler := sm.handlers[sm.state][args.e.Type]
			if args.ch != nil {
				if handler == nil {
					args.ch <- ErrIllegalEvent
					close(args.ch)
					continue
				}
				args.ch <- nil
				close(args.ch)
			}
			next := handler(sm.state, sm.ctx, args.e)
			sm.state = next
		case cmdTerminate:
			close(sm.terminatedCh)
			return
		default:
			panic("Unknown command received")
		}
	}
}

// ERRORS ---------------------------------------------------------------------

var (
	// Returned from Emit if there is no mapping for the current state and the
	// event that is being emitted.
	ErrIllegalEvent = errors.New("Illegal event received")

	// Returned from a method if the state machine is already terminated.
	ErrTerminated   = errors.New("State machine terminated")
)
