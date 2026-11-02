package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/sqlite"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func originalMediaFixture(t *testing.T) (serviceMessage, context.Context, domainMessage.OriginalMediaRequest, *domainChatStorage.Message, domainChatStorage.IChatStorageRepository) {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, filepath.Join(t.TempDir(), "original.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := chatstorage.NewStorageRepository(db)
	if err = repo.InitializeSchema(); err != nil {
		t.Fatal(err)
	}
	device := types.NewJID("15550000001", types.DefaultUserServer)
	chat := "15550000002@s.whatsapp.net"
	client := &whatsmeow.Client{Store: &store.Device{ID: &device}}
	ctx := whatsapp.ContextWithDevice(context.Background(), whatsapp.NewDeviceInstance("slot-a", client, repo))
	content := []byte("synthetic original bytes")
	digest := sha256.Sum256(content)
	row := &domainChatStorage.Message{ID: "MSG_ORIGINAL", DeviceID: device.String(), ChatJID: chat,
		Sender: chat, Timestamp: time.Now(), MediaType: "audio", URL: "https://mmg.whatsapp.net/v/test.enc?token=synthetic",
		MediaKey: bytes.Repeat([]byte{1}, 32), FileSHA256: digest[:], FileEncSHA256: bytes.Repeat([]byte{2}, 32), FileLength: uint64(len(content))}
	if err = repo.StoreChat(&domainChatStorage.Chat{DeviceID: row.DeviceID, JID: chat, Name: "synthetic", LastMessageTime: row.Timestamp}); err != nil {
		t.Fatal(err)
	}
	if err = repo.StoreMessage(row); err != nil {
		t.Fatal(err)
	}
	request := domainMessage.OriginalMediaRequest{MessageID: row.ID, ChatJID: chat, ExpectedDeviceJID: device.String(), ExpectedSHA256: hex.EncodeToString(digest[:]), MimeType: "audio/ogg"}
	service := serviceMessage{chatStorageRepo: repo, downloadOriginal: func(ctx context.Context, _ *whatsmeow.Client, msg whatsmeow.DownloadableMessage, file whatsmeow.File) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > OriginalMediaTimeout {
			t.Fatal("missing bounded SDK deadline")
		}
		if msg.GetDirectPath() != "/v/test.enc?token=synthetic" {
			t.Fatal("unexpected stored direct path")
		}
		_, err := file.Write(content)
		return err
	}}
	return service, ctx, request, row, repo
}

func TestOriginalMediaExactSQLiteScopeAndBytes(t *testing.T) {
	service, ctx, request, row, repo := originalMediaFixture(t)
	// Same ID under another device/chat must never supply this request's row.
	other := *row
	other.DeviceID = "15550000003@s.whatsapp.net"
	other.FileSHA256 = bytes.Repeat([]byte{9}, 32)
	if err := repo.StoreChat(&domainChatStorage.Chat{DeviceID: other.DeviceID, JID: other.ChatJID, LastMessageTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := repo.StoreMessage(&other); err != nil {
		t.Fatal(err)
	}
	response, err := service.OriginalMedia(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Content) != "synthetic original bytes" || response.DeviceJID != row.DeviceID || response.ChatJID != row.ChatJID || response.MessageID != row.ID || response.SHA256 != request.ExpectedSHA256 {
		t.Fatalf("wrong original: %+v", response)
	}
	if err = repo.DeleteMessageByDevice(row.DeviceID, row.ID, row.ChatJID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.OriginalMedia(ctx, request); !errors.Is(err, domainMessage.ErrOriginalMediaUnavailable) {
		t.Fatalf("must not fall back to other device: %v", err)
	}
}

func TestOriginalMediaRejectsChangedIdentityAndDigest(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domainMessage.OriginalMediaRequest, *domainChatStorage.Message)
		want   error
	}{
		{"wrong device", func(r *domainMessage.OriginalMediaRequest, _ *domainChatStorage.Message) {
			r.ExpectedDeviceJID = "15550000009@s.whatsapp.net"
		}, domainMessage.ErrOriginalMediaScope},
		{"wrong chat", func(r *domainMessage.OriginalMediaRequest, _ *domainChatStorage.Message) {
			r.ChatJID = "15550000009@s.whatsapp.net"
		}, domainMessage.ErrOriginalMediaUnavailable},
		{"missing hash", func(r *domainMessage.OriginalMediaRequest, _ *domainChatStorage.Message) { r.ExpectedSHA256 = "" }, domainMessage.ErrOriginalMediaInvalid},
		{"LID", func(r *domainMessage.OriginalMediaRequest, _ *domainChatStorage.Message) { r.ChatJID = "123@lid" }, domainMessage.ErrOriginalMediaInvalid},
		{"URL identity", func(r *domainMessage.OriginalMediaRequest, _ *domainChatStorage.Message) {
			r.MessageID = "https://127.0.0.1/"
		}, domainMessage.ErrOriginalMediaInvalid},
		{"edited digest", func(_ *domainMessage.OriginalMediaRequest, row *domainChatStorage.Message) {
			row.FileSHA256 = bytes.Repeat([]byte{9}, 32)
		}, domainMessage.ErrOriginalMediaIntegrity},
		{"oversized declaration", func(_ *domainMessage.OriginalMediaRequest, row *domainChatStorage.Message) {
			row.FileLength = uint64(OriginalMediaMaxBytes + 1)
		}, domainMessage.ErrOriginalMediaTooLarge},
		{"outgoing row", func(_ *domainMessage.OriginalMediaRequest, row *domainChatStorage.Message) { row.IsFromMe = true }, domainMessage.ErrOriginalMediaScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, ctx, request, row, repo := originalMediaFixture(t)
			tc.mutate(&request, row)
			if err := repo.StoreMessage(row); err != nil {
				t.Fatal(err)
			}
			service.downloadOriginal = func(context.Context, *whatsmeow.Client, whatsmeow.DownloadableMessage, whatsmeow.File) error {
				t.Fatal("invalid scope reached SDK")
				return nil
			}
			if _, err := service.OriginalMedia(ctx, request); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestOriginalMediaRechecksAfterSDKWait(t *testing.T) {
	for _, mode := range []string{"changed row", "slot rebound", "bad bytes", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			service, ctx, request, row, repo := originalMediaFixture(t)
			service.downloadOriginal = func(ctx context.Context, client *whatsmeow.Client, _ whatsmeow.DownloadableMessage, file whatsmeow.File) error {
				if mode == "timeout" {
					return context.DeadlineExceeded
				}
				content := []byte("synthetic original bytes")
				if mode == "bad bytes" {
					content[0] = 'X'
				}
				if _, err := file.Write(content); err != nil {
					return err
				}
				if mode == "changed row" {
					row.FileSHA256 = bytes.Repeat([]byte{3}, 32)
					return repo.StoreMessage(row)
				}
				if mode == "slot rebound" {
					jid := types.NewJID("15550000099", types.DefaultUserServer)
					client.Store.ID = &jid
				}
				return nil
			}
			if _, err := service.OriginalMedia(ctx, request); err == nil {
				t.Fatal("changed original was returned")
			}
		})
	}
}

func TestBoundedOriginalFileNeverBypassesWriteLimit(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "bounded")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	bounded := &boundedOriginalFile{file: file, limit: 32, ctx: context.Background()}
	if _, ok := any(bounded).(io.ReaderFrom); ok {
		t.Fatal("ReaderFrom bypasses Write")
	}
	if _, err = io.Copy(bounded, bytes.NewBuffer(make([]byte, 33))); !errors.Is(err, domainMessage.ErrOriginalMediaTooLarge) {
		t.Fatalf("unbounded copy: %v", err)
	}
	if _, err = bounded.WriteAt([]byte{1}, 32); !errors.Is(err, domainMessage.ErrOriginalMediaTooLarge) {
		t.Fatal(err)
	}
	if err = bounded.Truncate(33); !errors.Is(err, domainMessage.ErrOriginalMediaTooLarge) {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil || info.Size() > 32 || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe temporary file: %v %v", info, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bounded.ctx = ctx
	if _, err = bounded.Write([]byte{1}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
