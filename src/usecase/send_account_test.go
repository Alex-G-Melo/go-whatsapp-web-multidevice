package usecase

import (
	"context"
	"errors"
	"testing"

	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func TestSendTextRejectsWrongPhysicalAccountBeforeRecipientLookup(t *testing.T) {
	jid := types.NewJID("15550000001", types.DefaultUserServer)
	client := &whatsmeow.Client{Store: &store.Device{ID: &jid}}
	ctx := whatsapp.ContextWithDevice(context.Background(), whatsapp.NewDeviceInstance("slot-uuid", client, nil))
	service := serviceSend{}
	_, err := service.SendText(ctx, domainSend.MessageRequest{BaseRequest: domainSend.BaseRequest{
		Phone: "15550000002", ExpectedDeviceJID: "15550000009@s.whatsapp.net"}, Message: "synthetic"})
	if !errors.Is(err, pkgError.ErrSendAccountMismatch) {
		t.Fatalf("wrong physical account should fail before disconnected SDK recipient lookup: %v", err)
	}
}

func TestMediaUploadRejectsReassignedSelectedClientBeforeSDK(t *testing.T) {
	jid := types.NewJID("15550000001", types.DefaultUserServer)
	client := &whatsmeow.Client{Store: &store.Device{ID: &jid}}
	instance := whatsapp.NewDeviceInstance("slot-uuid", client, nil)
	ctx, err := whatsapp.ContextWithSendAccount(whatsapp.ContextWithDevice(context.Background(), instance), client, jid.String())
	if err != nil {
		t.Fatal(err)
	}
	other := types.NewJID("15550000009", types.DefaultUserServer)
	instance.SetClient(&whatsmeow.Client{Store: &store.Device{ID: &other}})
	service := serviceSend{}
	_, err = service.uploadMedia(ctx, client, whatsmeow.MediaImage, []byte("synthetic"), types.NewJID("15550000002", types.DefaultUserServer))
	if !errors.Is(err, pkgError.ErrSendAccountMismatch) {
		t.Fatalf("reassigned client reached upload SDK: %v", err)
	}
}

func TestMediaSendEntrypointsRejectWrongPhysicalAccountBeforePreparation(t *testing.T) {
	jid := types.NewJID("15550000001", types.DefaultUserServer)
	client := &whatsmeow.Client{Store: &store.Device{ID: &jid}}
	ctx := whatsapp.ContextWithDevice(context.Background(), whatsapp.NewDeviceInstance("slot-uuid", client, nil))
	base := domainSend.BaseRequest{Phone: "15550000002", ExpectedDeviceJID: "15550000009@s.whatsapp.net"}
	url := "https://example.invalid/synthetic"
	service := serviceSend{}
	cases := map[string]func() (domainSend.GenericResponse, error){
		"image": func() (domainSend.GenericResponse, error) {
			return service.SendImage(ctx, domainSend.ImageRequest{BaseRequest: base, ImageURL: &url})
		},
		"file": func() (domainSend.GenericResponse, error) {
			return service.SendFile(ctx, domainSend.FileRequest{BaseRequest: base, FileURL: &url})
		},
		"video": func() (domainSend.GenericResponse, error) {
			return service.SendVideo(ctx, domainSend.VideoRequest{BaseRequest: base, VideoURL: &url})
		},
		"audio": func() (domainSend.GenericResponse, error) {
			return service.SendAudio(ctx, domainSend.AudioRequest{BaseRequest: base, AudioURL: &url})
		},
		"sticker": func() (domainSend.GenericResponse, error) {
			return service.SendSticker(ctx, domainSend.StickerRequest{BaseRequest: base, StickerURL: &url})
		},
	}
	for name, send := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := send(); !errors.Is(err, pkgError.ErrSendAccountMismatch) {
				t.Fatalf("wrong account reached media preparation or recipient SDK: %v", err)
			}
		})
	}
}
