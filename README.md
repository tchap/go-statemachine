# go-statemachine

[![Build Status](https://travis-ci.org/tchap/go-statemachine.png?branch=master)](https://travis-ci.org/tchap/go-statemachine)

Transform your Go structs into tiny state machines.

## About

In Go you quite often need to make your struct methods thread-safe and also allow only particular sequences of calls, e.g. when `Close` is called, no other method can be called ever again. And this is exactly what go-statemachine is handling for your. If does not use any mutexes, just channels. I might not be as fast as mutexes, but it's nice and robust.

## State of the Project

I am still developing this, so things may and will change if I find it more
appropriate for my use cases.

## Example

Check `ExampleStateMachine` in `statemachine_test.go` to see an example.

## Documentation

We are writing Go, so [GoDoc](http://godoc.org/github.com/tchap/go-statemachine), what were you expecting?

## License

MIT
