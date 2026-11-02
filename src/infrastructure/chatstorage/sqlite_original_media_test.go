package chatstorage

import (
	"testing"
	"time"
)

func TestOriginalLookupRequiresExactDeviceChatAndMessage(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	for _, device := range []string{"15550000001@s.whatsapp.net", "15550000002@s.whatsapp.net"} {
		for _, chat := range []string{"15550000003@s.whatsapp.net", "15550000004@s.whatsapp.net"} {
			seedChatMessage(t, repo, device, chat, "SAME_ID", device+":"+chat, time.Now())
		}
	}
	device, chat := "15550000001@s.whatsapp.net", "15550000004@s.whatsapp.net"
	row, err := repo.GetMessageByIDChatAndDevice(device, chat, "SAME_ID")
	if err != nil || row == nil || row.Content != device+":"+chat {
		t.Fatalf("wrong exact scope: %+v %v", row, err)
	}
	row, err = repo.GetMessageByIDChatAndDevice("15550000009@s.whatsapp.net", chat, "SAME_ID")
	if err != nil || row != nil {
		t.Fatalf("cross-device fallback: %+v %v", row, err)
	}
	if _, err = repo.GetMessageByIDChatAndDevice("", chat, "SAME_ID"); err == nil {
		t.Fatal("missing device accepted")
	}
	if _, err = repo.GetMessageByIDChatAndDevice(device, "", "SAME_ID"); err == nil {
		t.Fatal("missing chat accepted")
	}
}
