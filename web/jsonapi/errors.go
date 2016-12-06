package jsonapi

import (
	"fmt"
	"net/http"
	"strconv"
)

// SourceError contains references to the source of the error
type SourceError struct {
	Pointer   string `json:"pointer,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

// Error objects provide additional information about problems encountered
// while performing an operation.
// See http://jsonapi.org/format/#error-objects
type Error struct {
	Status int         `json:"status,string"`
	Title  string      `json:"title"`
	Detail string      `json:"detail"`
	Source SourceError `json:"source,omitempty"`
}

// ErrorList is just an array of error objects
type ErrorList []*Error

func (e *Error) Error() string {
	return e.Title + "(" + strconv.Itoa(e.Status) + ")" + ": " + e.Detail
}

// NewError creates a new generic Error
func NewError(status int, msg ...interface{}) *Error {
	je := &Error{
		Status: status,
		Title:  http.StatusText(status),
	}
	if len(msg) > 0 {
		je.Detail = fmt.Sprint(msg...)
	}
	return je
}

// NotFound returns a 404 formatted error
func NotFound(err error) *Error {
	return &Error{
		Status: http.StatusNotFound,
		Title:  "Not Found",
		Detail: err.Error(),
	}
}

// BadRequest returns a 400 formatted error
func BadRequest(err error) *Error {
	return &Error{
		Status: http.StatusBadRequest,
		Title:  "Bad request",
		Detail: err.Error(),
	}
}

// BadJSON returns a 400 formatted error meaning the json input is
// malformed.
func BadJSON() *Error {
	return &Error{
		Status: http.StatusBadRequest,
		Title:  "Bad request",
		Detail: "JSON input is malformed or is missing mandatory fields",
	}
}

// Conflict returns a 409 formatted error representing a conflict
func Conflict(err error) *Error {
	return &Error{
		Status: http.StatusConflict,
		Title:  "Conflict",
		Detail: err.Error(),
	}
}

// InternalServerError returns a 500 formatted error
func InternalServerError(err error) *Error {
	return &Error{
		Status: http.StatusInternalServerError,
		Title:  "Internal Server Error",
		Detail: err.Error(),
	}
}

// PreconditionFailed returns a 412 formatted error when an expectation from an
// HTTP header is not matched
func PreconditionFailed(parameter string, err error) *Error {
	return &Error{
		Status: http.StatusPreconditionFailed,
		Title:  "Precondition Failed",
		Detail: err.Error(),
		Source: SourceError{
			Parameter: parameter,
		},
	}
}

// InvalidParameter returns a 422 formatted error when an HTTP or Query-String
// parameter is invalid
func InvalidParameter(parameter string, err error) *Error {
	return &Error{
		Status: http.StatusUnprocessableEntity,
		Title:  "Invalid Parameter",
		Detail: err.Error(),
		Source: SourceError{
			Parameter: parameter,
		},
	}
}

// InvalidAttribute returns a 422 formatted error when an attribute is invalid
func InvalidAttribute(attribute string, err error) *Error {
	return &Error{
		Status: http.StatusUnprocessableEntity,
		Title:  "Invalid Attribute",
		Detail: err.Error(),
		Source: SourceError{
			Pointer: "/data/attributes/" + attribute,
		},
	}
}
