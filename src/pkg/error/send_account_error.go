package error

import "net/http"

type SendAccountError string

func (e SendAccountError) Error() string   { return string(e) }
func (e SendAccountError) ErrCode() string { return string(e) }
func (e SendAccountError) StatusCode() int {
	if e == ErrSendAccountOutcomeUnknown {
		return http.StatusServiceUnavailable
	}
	if e == ErrSendAccountInvalid {
		return http.StatusUnprocessableEntity
	}
	return http.StatusConflict
}

const (
	ErrSendAccountInvalid        = SendAccountError("SEND_ACCOUNT_INVALID")
	ErrSendAccountMismatch       = SendAccountError("SEND_ACCOUNT_MISMATCH")
	ErrSendAccountOutcomeUnknown = SendAccountError("SEND_ACCOUNT_OUTCOME_UNKNOWN")
)
