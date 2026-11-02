package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

const OriginalMediaMaxBytes int64 = 25 * 1024 * 1024
const OriginalMediaTimeout = 20 * time.Second

var originalPNJID = regexp.MustCompile(`^[0-9]{1,20}@s\.whatsapp\.net$`)
var originalMessageID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// OriginalMedia never creates a public/static copy and requires the admitted digest.
func (service serviceMessage) OriginalMedia(ctx context.Context, request domainMessage.OriginalMediaRequest) (response domainMessage.OriginalMediaResponse, err error) {
	expectedHash, err := hex.DecodeString(request.ExpectedSHA256)
	mediaMIME, _, mimeErr := mime.ParseMediaType(request.MimeType)
	if !originalMessageID.MatchString(request.MessageID) || !originalPNJID.MatchString(request.ChatJID) ||
		!originalPNJID.MatchString(request.ExpectedDeviceJID) || len(expectedHash) != sha256.Size || err != nil ||
		mimeErr != nil || len(request.MimeType) > 100 || strings.ContainsAny(request.MimeType, "\r\n") {
		return response, domainMessage.ErrOriginalMediaInvalid
	}
	instance, ok := whatsapp.DeviceFromContext(ctx)
	if !ok || instance == nil {
		return response, domainMessage.ErrOriginalMediaScope
	}
	client := instance.GetClient()
	if client == nil || client.Store == nil || client.Store.ID == nil || client.Store.ID.ToNonAD().String() != request.ExpectedDeviceJID || service.chatStorageRepo == nil {
		return response, domainMessage.ErrOriginalMediaScope
	}
	ctx, cancel := context.WithTimeout(ctx, OriginalMediaTimeout)
	defer cancel()
	message, err := service.originalMediaRow(ctx, request, expectedHash)
	if err != nil {
		return response, err
	}
	downloadable, err := originalDownloadable(message)
	if err != nil {
		return response, err
	}
	temp, err := os.CreateTemp("", "gowa-private-original-*")
	if err != nil {
		return response, err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	// The SDK writes ciphertext first: at most one padding block plus a 10-byte MAC.
	file := &boundedOriginalFile{file: temp, limit: OriginalMediaMaxBytes + 26, ctx: ctx}
	if service.downloadOriginal != nil {
		err = service.downloadOriginal(ctx, client, downloadable, file)
	} else {
		err = client.DownloadToFile(ctx, downloadable, file)
	}
	if err != nil {
		return response, err
	}
	if err = ctx.Err(); err != nil {
		return response, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return response, err
	}
	content, err := io.ReadAll(io.LimitReader(file, OriginalMediaMaxBytes+1))
	if err != nil {
		return response, err
	}
	actualHash := sha256.Sum256(content)
	if uint64(len(content)) != message.FileLength || !hmac.Equal(actualHash[:], expectedHash) {
		return response, domainMessage.ErrOriginalMediaIntegrity
	}
	// Re-read after the external wait; an edited/deleted row or a reused slot is not this original.
	current, err := service.originalMediaRow(ctx, request, expectedHash)
	if err != nil {
		return response, err
	}
	if instance.GetClient() != client || client.Store == nil || client.Store.ID == nil || client.Store.ID.ToNonAD().String() != request.ExpectedDeviceJID ||
		current.FileLength != message.FileLength || current.MediaType != message.MediaType {
		return response, domainMessage.ErrOriginalMediaScope
	}
	return domainMessage.OriginalMediaResponse{Content: content, DeviceJID: request.ExpectedDeviceJID,
		ChatJID: request.ChatJID, MessageID: request.MessageID, SHA256: hex.EncodeToString(actualHash[:]),
		MediaType: message.MediaType, MimeType: mediaMIME}, nil
}

func (service serviceMessage) originalMediaRow(ctx context.Context, request domainMessage.OriginalMediaRequest, expectedHash []byte) (*domainChatStorage.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	row, err := service.chatStorageRepo.GetMessageByIDChatAndDevice(request.ExpectedDeviceJID, request.ChatJID, request.MessageID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, domainMessage.ErrOriginalMediaUnavailable
	}
	if row.DeviceID != request.ExpectedDeviceJID || row.ChatJID != request.ChatJID || row.ID != request.MessageID ||
		row.Sender != request.ChatJID || row.IsFromMe {
		return nil, domainMessage.ErrOriginalMediaScope
	}
	if row.FileLength == 0 || row.FileLength > uint64(OriginalMediaMaxBytes) {
		return nil, domainMessage.ErrOriginalMediaTooLarge
	}
	if len(row.MediaKey) != 32 || len(row.FileEncSHA256) != sha256.Size || !hmac.Equal(row.FileSHA256, expectedHash) {
		return nil, domainMessage.ErrOriginalMediaIntegrity
	}
	return row, nil
}

func originalDownloadable(row *domainChatStorage.Message) (whatsmeow.DownloadableMessage, error) {
	// Ignore the stored URL host: the SDK selects WhatsApp's authenticated media hosts.
	location, err := url.Parse(row.URL)
	if err != nil || location.Scheme != "https" || location.Host == "" || location.User != nil || location.Fragment != "" ||
		!strings.HasPrefix(location.Path, "/") || strings.HasPrefix(location.Path, "//") || location.RawQuery == "" {
		return nil, domainMessage.ErrOriginalMediaUnavailable
	}
	directPath := location.EscapedPath() + "?" + location.RawQuery
	switch row.MediaType {
	case "image":
		return &waE2E.ImageMessage{DirectPath: proto.String(directPath), MediaKey: row.MediaKey, FileSHA256: row.FileSHA256, FileEncSHA256: row.FileEncSHA256, FileLength: proto.Uint64(row.FileLength)}, nil
	case "audio":
		return &waE2E.AudioMessage{DirectPath: proto.String(directPath), MediaKey: row.MediaKey, FileSHA256: row.FileSHA256, FileEncSHA256: row.FileEncSHA256, FileLength: proto.Uint64(row.FileLength)}, nil
	case "document":
		return &waE2E.DocumentMessage{DirectPath: proto.String(directPath), MediaKey: row.MediaKey, FileSHA256: row.FileSHA256, FileEncSHA256: row.FileEncSHA256, FileLength: proto.Uint64(row.FileLength)}, nil
	default:
		return nil, domainMessage.ErrOriginalMediaUnavailable
	}
}

// No os.File embedding: io.Copy must not find its ReaderFrom and bypass Write's bound.
type boundedOriginalFile struct {
	file  *os.File
	limit int64
	ctx   context.Context
}

func (f *boundedOriginalFile) Read(p []byte) (int, error)              { return f.file.Read(p) }
func (f *boundedOriginalFile) ReadAt(p []byte, off int64) (int, error) { return f.file.ReadAt(p, off) }
func (f *boundedOriginalFile) Stat() (os.FileInfo, error)              { return f.file.Stat() }
func (f *boundedOriginalFile) Seek(off int64, whence int) (int64, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	position, err := f.file.Seek(off, whence)
	if err == nil && (position < 0 || position > f.limit) {
		return 0, domainMessage.ErrOriginalMediaTooLarge
	}
	return position, err
}
func (f *boundedOriginalFile) Write(p []byte) (int, error) {
	position, err := f.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return f.write(p, position, false)
}
func (f *boundedOriginalFile) WriteAt(p []byte, off int64) (int, error) { return f.write(p, off, true) }
func (f *boundedOriginalFile) write(p []byte, off int64, at bool) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off > f.limit || int64(len(p)) > f.limit-off {
		return 0, domainMessage.ErrOriginalMediaTooLarge
	}
	if at {
		return f.file.WriteAt(p, off)
	}
	return f.file.Write(p)
}
func (f *boundedOriginalFile) Truncate(size int64) error {
	if err := f.ctx.Err(); err != nil {
		return err
	}
	if size < 0 || size > f.limit {
		return domainMessage.ErrOriginalMediaTooLarge
	}
	return f.file.Truncate(size)
}

var _ whatsmeow.File = (*boundedOriginalFile)(nil)
