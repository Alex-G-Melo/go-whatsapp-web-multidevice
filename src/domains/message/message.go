package message

import "errors"

type GenericResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

type RevokeRequest struct {
	MessageID string `json:"message_id" uri:"message_id"`
	Phone     string `json:"phone" form:"phone"`
}

type DeleteRequest struct {
	MessageID string `json:"message_id" uri:"message_id"`
	Phone     string `json:"phone" form:"phone"`
}

type ReactionRequest struct {
	MessageID string `json:"message_id" form:"message_id"`
	Phone     string `json:"phone" form:"phone"`
	Emoji     string `json:"emoji" form:"emoji"`
}

type UpdateMessageRequest struct {
	MessageID string `json:"message_id" uri:"message_id"`
	Message   string `json:"message" form:"message"`
	Phone     string `json:"phone" form:"phone"`
}

type MarkAsReadRequest struct {
	MessageID string `json:"message_id" uri:"message_id"`
	Phone     string `json:"phone" form:"phone"`
}

type StarRequest struct {
	MessageID string `json:"message_id" uri:"message_id"`
	Phone     string `json:"phone" form:"phone"`
	IsStarred bool   `json:"is_starred"`
}

type DownloadMediaRequest struct {
	MessageID string `json:"message_id" uri:"message_id"`
	Phone     string `json:"phone" form:"phone"`
}

type DownloadMediaResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
	MediaType string `json:"media_type"`
	Filename  string `json:"filename"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
}

// OriginalMediaRequest deliberately accepts no URL, path or mutable customer alias.
type OriginalMediaRequest struct {
	MessageID         string
	ChatJID           string
	ExpectedDeviceJID string
	ExpectedSHA256    string
	MimeType          string
}

type OriginalMediaResponse struct {
	Content   []byte
	DeviceJID string
	ChatJID   string
	MessageID string
	SHA256    string
	MediaType string
	MimeType  string
}

const OriginalMediaContract = "gowa-original-1"

var ErrOriginalMediaInvalid = errors.New("original_media_identity_invalid")
var ErrOriginalMediaUnavailable = errors.New("original_media_unavailable")
var ErrOriginalMediaScope = errors.New("original_media_scope_changed")
var ErrOriginalMediaIntegrity = errors.New("original_media_integrity_failed")
var ErrOriginalMediaTooLarge = errors.New("original_media_oversized")
