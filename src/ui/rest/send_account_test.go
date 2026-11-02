package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/gofiber/fiber/v2"
)

type physicalSendHTTPService struct {
	domainSend.ISendUsecase
	base domainSend.BaseRequest
}

func (s *physicalSendHTTPService) SendText(_ context.Context, request domainSend.MessageRequest) (domainSend.GenericResponse, error) {
	s.base = request.BaseRequest
	return domainSend.GenericResponse{MessageID: "SYNTHETIC", DeviceJID: request.ExpectedDeviceJID}, nil
}

func (s *physicalSendHTTPService) SendImage(_ context.Context, request domainSend.ImageRequest) (domainSend.GenericResponse, error) {
	s.base = request.BaseRequest
	return domainSend.GenericResponse{MessageID: "SYNTHETIC", DeviceJID: request.ExpectedDeviceJID}, nil
}

func TestPhysicalSendHTTPJSONAndFormContract(t *testing.T) {
	for _, mode := range []string{"json", "multipart", "omitted"} {
		t.Run(mode, func(t *testing.T) {
			service := &physicalSendHTTPService{}
			app := fiber.New()
			InitRestSend(app, service)
			expected := "15550000001@s.whatsapp.net"
			body, path, contentType := bytes.NewBuffer(nil), "/send/message", "application/json"
			if mode == "multipart" {
				path = "/send/image"
				writer := multipart.NewWriter(body)
				for field, value := range map[string]string{"phone": "15550000002", "image_url": "https://example.invalid/synthetic.png", "expected_device_jid": expected} {
					if err := writer.WriteField(field, value); err != nil {
						t.Fatal(err)
					}
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				contentType = writer.FormDataContentType()
			} else {
				payload := map[string]string{"phone": "15550000002", "message": "synthetic"}
				if mode == "json" {
					payload["expected_device_jid"] = expected
				} else {
					expected = ""
				}
				if err := json.NewEncoder(body).Encode(payload); err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest("POST", path, body)
			request.Header.Set("Content-Type", contentType)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var result struct {
				Results domainSend.GenericResponse `json:"results"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 || service.base.ExpectedDeviceJID != expected || result.Results.DeviceJID != expected {
				t.Fatalf("lost physical precondition: status=%d request=%+v response=%+v", response.StatusCode, service.base, result)
			}
			wantContract := domainSend.PhysicalSendContract
			if expected == "" {
				wantContract = ""
			}
			if response.Header.Get("X-GoWA-Send-Contract") != wantContract {
				t.Fatal("unexpected physical send contract header")
			}
		})
	}
}
