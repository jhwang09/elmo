package errs

import (
	"fmt"
	"runtime/debug"
	"time"
)

type Err interface {
	Message() string
	StdError() error
	Stack() []byte
	Info() Info
	Time() time.Time
	StdErrorMessage() string
}

type Info map[string]interface{}

func NewError(message string, info Info) Err {
	return newErr(message, nil, debug.Stack(), info)
}

func NewStdError(stdErr error, info Info) Err {
	return newErr("", stdErr, debug.Stack(), info)
}

func newErr(message string, stdErr error, stack []byte, info Info) Err {
	if info == nil {
		info = Info{}
	}
	return &err{message, stdErr, stack, info, time.Now()}
}

type err struct {
	message string
	stdErr  error
	stack   []byte
	info    Info
	time    time.Time
}

func (e *err) Message() string { return e.message }
func (e *err) StdError() error { return e.stdErr }
func (e *err) Stack() []byte   { return e.stack }
func (e *err) Info() Info      { return e.info }
func (e *err) Time() time.Time { return e.time }
func (e *err) StdErrorMessage() string {
	if e == nil || e.stdErr == nil {
		return ""
	}
	return e.stdErr.Error()
}

func (e *err) String() string {
	if e.stdErr == nil {
		return fmt.Sprint("Error | ", e.time, " | Error: ", e.message, " | ", string(e.stack), " | info:[", e.info, "]")
	} else {
		return fmt.Sprint("Error | ", e.time, " | StdError: ", e.StdErrorMessage(), " | ", string(e.stack), " | info:[", e.info, "]")
	}
}
