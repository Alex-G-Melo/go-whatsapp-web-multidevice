package whatsapp

import (
	"context"
	"errors"
	"testing"

	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func sendAccountFixture() (context.Context, *DeviceInstance, *whatsmeow.Client, string) {
	jid := types.NewJID("15550000001", types.DefaultUserServer)
	client := &whatsmeow.Client{Store: &store.Device{ID: &jid}}
	instance := NewDeviceInstance("11111111-1111-4111-8111-111111111111", client, nil)
	return ContextWithDevice(context.Background(), instance), instance, client, jid.String()
}

func TestPhysicalSendAccountExactSelectedClientAndOmission(t *testing.T) {
	before := sendMessageFn
	t.Cleanup(func() { sendMessageFn = before })
	ctx, instance, client, expected := sendAccountFixture()
	if instance.ID() == expected {
		t.Fatal("fixture must distinguish the selected slot UUID from its physical JID")
	}
	calls := 0
	sendMessageFn = func(_ context.Context, got *whatsmeow.Client, _ types.JID, _ *waE2E.Message) (whatsmeow.SendResponse, error) {
		calls++
		if got != client {
			t.Fatal("send must use the captured selected client")
		}
		return whatsmeow.SendResponse{ID: "SYNTHETIC"}, nil
	}
	for _, binding := range []string{expected, ""} {
		bound, err := ContextWithSendAccount(ctx, client, binding)
		if err != nil {
			t.Fatal(err)
		}
		response, err := SendMessageWithReachoutRetry(bound, client, types.NewJID("15550000002", types.DefaultUserServer), &waE2E.Message{})
		if err != nil || response.ID != "SYNTHETIC" {
			t.Fatalf("unexpected send result %+v: %v", response, err)
		}
	}
	// Omitted expectations retain the old no-explicit-device compatibility.
	if _, err := ContextWithSendAccount(context.Background(), nil, ""); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("got %d SDK sends", calls)
	}
}

func TestPhysicalSendAccountRejectsInvalidOrWrongAccountBeforeSDK(t *testing.T) {
	before := sendMessageFn
	t.Cleanup(func() { sendMessageFn = before })
	sendMessageFn = func(context.Context, *whatsmeow.Client, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error) {
		t.Fatal("rejected expectation reached SDK send")
		return whatsmeow.SendResponse{}, nil
	}
	for _, expected := range []string{"15550000009@s.whatsapp.net", "15550000001:3@s.whatsapp.net", "15550000001@lid", " 15550000001@s.whatsapp.net", "slot-uuid", "15550000001@s.whatsapp.net\n"} {
		t.Run(expected, func(t *testing.T) {
			ctx, _, client, _ := sendAccountFixture()
			_, err := ContextWithSendAccount(ctx, client, expected)
			want := pkgError.ErrSendAccountInvalid
			if expected == "15550000009@s.whatsapp.net" {
				want = pkgError.ErrSendAccountMismatch
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
		})
	}
	_, _, client, expected := sendAccountFixture()
	if _, err := ContextWithSendAccount(context.Background(), client, expected); !errors.Is(err, pkgError.ErrSendAccountMismatch) {
		t.Fatalf("bound sends must not fall back to an ambient client: %v", err)
	}
}

func TestPhysicalSendAccountReassignmentAndIdentityChangeBeforeSDK(t *testing.T) {
	before := sendMessageFn
	t.Cleanup(func() { sendMessageFn = before })
	calls := 0
	sendMessageFn = func(context.Context, *whatsmeow.Client, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error) {
		calls++
		return whatsmeow.SendResponse{}, nil
	}
	for _, change := range []string{"reassigned slot", "same physical new client", "changed physical", "missing identity", "wrong supplied client"} {
		t.Run(change, func(t *testing.T) {
			ctx, instance, client, expected := sendAccountFixture()
			bound, err := ContextWithSendAccount(ctx, client, expected)
			if err != nil {
				t.Fatal(err)
			}
			otherJID := types.NewJID("15550000009", types.DefaultUserServer)
			other := &whatsmeow.Client{Store: &store.Device{ID: &otherJID}}
			switch change {
			case "reassigned slot":
				instance.SetClient(other)
			case "same physical new client":
				instance.SetClient(&whatsmeow.Client{Store: client.Store})
			case "changed physical":
				client.Store.ID = &otherJID
			case "missing identity":
				client.Store.ID = nil
			case "wrong supplied client":
				client = other
			}
			if _, err = SendMessageWithReachoutRetry(bound, client, types.NewJID("15550000002", types.DefaultUserServer), &waE2E.Message{}); !errors.Is(err, pkgError.ErrSendAccountMismatch) {
				t.Fatalf("got %v, want mismatch", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("account drift caused %d SDK sends", calls)
	}
}

func TestPhysicalSendAccountRechecksAfterReachoutPrewarm(t *testing.T) {
	beforeSend, beforeSubscribe := sendMessageFn, subscribePresenceFn
	t.Cleanup(func() { sendMessageFn, subscribePresenceFn = beforeSend, beforeSubscribe })
	ctx, instance, client, expected := sendAccountFixture()
	bound, err := ContextWithSendAccount(ctx, client, expected)
	if err != nil {
		t.Fatal(err)
	}
	calls, subscribes := 0, 0
	sendMessageFn = func(_ context.Context, got *whatsmeow.Client, _ types.JID, _ *waE2E.Message) (whatsmeow.SendResponse, error) {
		calls++
		if got != client {
			t.Fatal("send changed selected client")
		}
		return whatsmeow.SendResponse{}, reachoutErr()
	}
	subscribePresenceFn = func(ctx context.Context, got *whatsmeow.Client, _ types.JID) error {
		subscribes++
		if got != client || CheckSendAccount(ctx, got) != nil {
			t.Fatal("prewarm lost account binding")
		}
		other := types.NewJID("15550000009", types.DefaultUserServer)
		instance.SetClient(&whatsmeow.Client{Store: &store.Device{ID: &other}})
		return nil
	}
	_, err = SendMessageWithReachoutRetry(bound, client, types.NewJID("15550000002", types.DefaultUserServer), &waE2E.Message{})
	if !errors.Is(err, pkgError.ErrSendAccountMismatch) || calls != 1 || subscribes != 1 {
		t.Fatalf("retry must not send after reassignment: sends=%d subscribes=%d error=%v", calls, subscribes, err)
	}
}
