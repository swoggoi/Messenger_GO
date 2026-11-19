package core_errors

import "errors"

var (
	ErrInvalidUsername    = errors.New("invalid username")
	ErrUserNotFound       = errors.New("user not found")
	ErrChatNotFound       = errors.New("chat not found")
	ErrMessageNotFound    = errors.New("message not found")
	ErrDuplicateUser      = errors.New("user already exists")
	ErrValidation         = errors.New("validation error")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrGroupNotFound      = errors.New("academic group not found")
	ErrForbidden          = errors.New("access denied")
	ErrNotChatMember      = errors.New("you are not a member of this chat")
)
