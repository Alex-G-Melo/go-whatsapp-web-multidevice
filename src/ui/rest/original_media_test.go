package rest

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
)

type originalHTTPService struct {
	domainMessage.IMessageUsecase
	calls   int
	request domainMessage.OriginalMediaRequest
}

func (s *originalHTTPService) OriginalMedia(_ context.Context, request domainMessage.OriginalMediaRequest) (domainMessage.OriginalMediaResponse, error) {
	s.calls++
	s.request = request
	return domainMessage.OriginalMediaResponse{Content: []byte("synthetic original"), DeviceJID: request.ExpectedDeviceJID,
		ChatJID: request.ChatJID, MessageID: request.MessageID, SHA256: request.ExpectedSHA256,
		MediaType: "audio", MimeType: "audio/ogg"}, nil
}

func TestOriginalMediaHTTPRequiresConfiguredAuthenticationAndExplicitDevice(t *testing.T) {
	before := config.AppBasicAuthCredential
	t.Cleanup(func() { config.AppBasicAuthCredential = before })
	config.AppBasicAuthCredential = []string{"synthetic:private-test"}
	service := &originalHTTPService{}
	app := fiber.New()
	app.Use(basicauth.New(basicauth.Config{Users: map[string]string{"synthetic": "private-test"}}))
	InitRestMessage(app, service)
	url := "/message/MSG/original?chat_jid=15550000002%40s.whatsapp.net&expected_device_jid=15550000001%40s.whatsapp.net&expected_sha256=abcd&mime_type=audio%2Fogg"
	for _, mode := range []string{"missing auth", "wrong auth", "missing device", "valid", "auth unconfigured"} {
		request := httptest.NewRequest("GET", url, nil)
		if mode != "missing auth" {
			request.SetBasicAuth("synthetic", "private-test")
		}
		if mode == "wrong auth" {
			request.SetBasicAuth("synthetic", "wrong")
		}
		if mode != "missing device" {
			request.Header.Set("X-Device-Id", "slot-a")
		}
		if mode == "auth unconfigured" {
			config.AppBasicAuthCredential = nil
		}
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if mode == "valid" {
			if response.StatusCode != 200 || string(body) != "synthetic original" || response.Header.Get("X-GoWA-Original-Contract") != domainMessage.OriginalMediaContract || response.Header.Get("X-GoWA-Device-JID") != "15550000001@s.whatsapp.net" || response.Header.Get("Cache-Control") != "private, no-store" {
				t.Fatalf("bad original response: %d %s", response.StatusCode, body)
			}
		} else if response.StatusCode == 200 {
			t.Fatalf("%s reached original bytes", mode)
		}
	}
	if service.calls != 1 || service.request.MimeType != "audio/ogg" {
		t.Fatalf("unexpected service requests: %+v", service)
	}
}
