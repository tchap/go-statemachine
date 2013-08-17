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

type Event struct {
	Type EventType
	Data interface{}
}

type EventHandler func(state State, ctx Context, evt *Event) (next State)

type StateMachine struct {
	state State
	ctx   Context

	handlers [][]EventHandler

	cmdCh        chan *command
	terminatedCh chan struct{}
}

// CONSTRUCTOR ----------------------------------------------------------------

func NewStateMachine(initState State, initCtx Context, stateCount, eventCount, mailboxSize uint) *StateMachine {
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

func (sm *StateMachine) IsHandlerDefined(s State, t EventType) (defined bool, err error) {
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

func (sm *StateMachine) Off(s State, t EventType) error {
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

func (sm *StateMachine) Emit(event *Event, replyCh chan error) error {
	return sm.send(&command{
		cmdEmit,
		&cmdEmitArgs{event, replyCh},
	})
}

// Terminate ------------------------------------------------------------------

func (sm *StateMachine) Terminate() error {
	return sm.send(&command{
		cmdTerminate,
		nil,
	})
}

func (sm *StateMachine) TerminatedChannel() chan struct{} {
	return sm.terminatedCh
}

// INTERNALS ------------------------------------------------------------------

func (sm *StateMachine) send(cmd *command) error {
	select {
	case sm.cmdCh <- cmd:
		return nil
	case <-sm.terminatedCh:
		return ErrTerminated
	}
}

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
	ErrIllegalEvent = errors.New("Illegal event received")
	ErrTerminated   = errors.New("State machine terminated")
)
