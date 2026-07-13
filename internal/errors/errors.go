package core_errors

import "errors"

var (
	ErrInvalidUsername = errors.New("invalid username")
	ErrUserNotFound    = errors.New("user not found")
	ErrChatNotFound    = errors.New("chat not found")
	ErrMessageNotFound = errors.New("message not found")
	ErrDuplicateUser   = errors.New("user already exists")
)
