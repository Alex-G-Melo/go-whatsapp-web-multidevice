package whatsapp

import (
	"bytes"
	"encoding/hex"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestOriginalMediaMetadataKeepsOnlyExactIncomingPNIdentity(t *testing.T) {
	device := types.NewJID("15550000001", types.DefaultUserServer)
	chat := types.NewJID("15550000002", types.DefaultUserServer)
	client := &whatsmeow.Client{Store: &store.Device{ID: &device}}
	msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{FileSHA256: bytes.Repeat([]byte{3}, 32), FileLength: proto.Uint64(20), Mimetype: proto.String("audio/ogg")}}
	for _, mode := range []string{"incoming PN", "LID chat", "outgoing", "different sender", "missing digest", "view once", "ephemeral wrapper"} {
		event := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: chat}, ID: "MSG"}, Message: msg}
		media := msg
		if mode == "LID chat" {
			event.Info.Chat = types.NewJID("123", types.HiddenUserServer)
		}
		if mode == "outgoing" {
			event.Info.IsFromMe = true
		}
		if mode == "different sender" {
			event.Info.Sender = device
		}
		if mode == "missing digest" {
			media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{FileLength: proto.Uint64(20), Mimetype: proto.String("audio/ogg")}}
			event.Message = media
		}
		if mode == "view once" {
			event.IsViewOnce = true
		}
		if mode == "ephemeral wrapper" {
			event.Message = &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: msg}}
		}
		payload := map[string]any{}
		buildOriginalMediaFields(client, event, media, payload)
		original, exists := payload["original_media"].(map[string]any)
		if exists != (mode == "incoming PN") {
			t.Fatalf("unexpected original metadata for %s: %+v", mode, original)
		}
		if exists && (original["device_jid"] != device.String() || original["chat_jid"] != chat.String() || original["sender_jid"] != chat.String() || original["message_id"] != "MSG" || original["sha256"] != hex.EncodeToString(bytes.Repeat([]byte{3}, 32))) {
			t.Fatalf("changed signed identity: %+v", original)
		}
	}
}
