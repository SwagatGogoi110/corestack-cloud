package protocol

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	HTTPStatus int
	Status     string
	Message    string
}

func (e *Error) Error() string { return e.Status + ": " + e.Message }

func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func AlreadyExists(msg string) *Error { return &Error{409, "ALREADY_EXISTS", msg}}
func InvalidArgument(msg string) *Error { return &Error{400, "INVALID_ARGUMENT", msg}}
func OutOfRange(msg string) *Error { return &Error{416, "OUT_OF_RANGE", msg}}
func FailedPreCondition(msg string) *Error { return &Error{400, "FAILED_PRECONDITION", msg}}
func ConditionNotMet(msg string) *Error { return &Error{412, "ConditionNotMet", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}
func NotFound(msg string) *Error { return &Error{404, "NOT_FOUND", msg}}