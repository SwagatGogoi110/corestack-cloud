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

func NotFound(msg string) *Error           { return &Error{404, "NOT_FOUND", msg} }
func AlreadyExists(msg string) *Error      { return &Error{409, "ALREADY_EXISTS", msg} }
func InvalidArgument(msg string) *Error    { return &Error{400, "INVALID_ARGUMENT", msg} }
func OutOfRange(msg string) *Error         { return &Error{416, "OUT_OF_RANGE", msg} }
func FailedPreCondition(msg string) *Error { return &Error{400, "FAILED_PRECONDITION", msg} }
func ConditionNotMet(msg string) *Error    { return &Error{412, "CONDITION_NOT_MET", msg} }
func PermissionDenied(msg string) *Error   { return &Error{403, "PERMISSION_DENIED", msg} }
func ResourceExhausted(msg string) *Error  { return &Error{429, "RESOURCE_EXHAUSTED", msg} }
func Unimplemented(msg string) *Error      { return &Error{501, "UNIMPLEMENTED", msg} }
func DeadlineExceeded(msg string) *Error   { return &Error{504, "DEADLINE_EXCEEDED", msg} }
func Unavailable(msg string) *Error        { return &Error{503, "UNAVAILABLE", msg} }
func Internal(msg string) *Error           { return &Error{500, "INTERNAL", msg} }
func BadGateway(msg string) *Error         { return &Error{502, "INTERNAL", msg} }

func reasonFor(status string) string {
	switch status {
	case "NOT_FOUND":
		return "notFound"
	case "ALREADY_EXISTS":
		return "alreadyExists"
	case "INVALID_ARGUMENT":
		return "invalid"
	case "OUT_OF_RANGE":
		return "outOfRange"
	case "FAILED_PRECONDITION":
		return "failedPrecondition"
	case "CONDITION_NOT_MET":
		return "conditionNotMet"
	case "PERMISSION_DENIED":
		return "forbidden"
	case "RESOURCE_EXHAUSTED":
		return "rateLimitExceeded"
	case "UNIMPLEMENTED":
		return "notImplemented"
	case "DEADLINE_EXCEEDED":
		return "deadlineExceeded"
	case "UNAVAILABLE":
		return "backendError"
	case "INTERNAL":
		return "internalError"
	default:
		return "backendError"
	}
}

func WriteError(w http.ResponseWriter, e *Error) {
	body := map[string]any{
		"error": map[string]any{
			"code":    e.HTTPStatus,
			"message": e.Message,
			"status":  e.Status,
			"errors": []any{map[string]any{
				"message": e.Message,
				"domain":  "global",
				"reason":  reasonFor(e.Status),
			}},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(body)
}

func AsError(err error) *Error {
	if e, ok := err.(*Error); ok {
		return e
	}
	return Internal(err.Error())
}
