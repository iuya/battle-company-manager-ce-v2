package core

import "errors"

// FieldError is a validation message that belongs to one input field of a form: the page shows it under that field. The message is
// worded for the page (lower case start, no full stop); Field is empty for a message about the form as a whole.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return e.Msg }

func fieldError(field, msg string) error { return &FieldError{Field: field, Msg: msg} }

// FieldOf is the form field an error is about, empty when it is not about one.
func FieldOf(err error) string {
	if fe, ok := errors.AsType[*FieldError](err); ok {
		return fe.Field
	}
	return ""
}
