package rest

import (
	"context"
	"errors"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/gofiber/fiber/v2"
)

// Registered inside the authenticated, device-scoped group only.
func (controller *Message) OriginalMedia(c *fiber.Ctx) error {
	if len(config.AppBasicAuthCredential) == 0 {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"code": "original_media_auth_unconfigured"})
	}
	if c.Get("Authorization") == "" || c.Get("X-Device-Id") == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"code": "original_media_explicit_auth_device_required"})
	}
	response, err := controller.Service.OriginalMedia(whatsapp.ContextWithDevice(c.UserContext(), getDeviceFromCtx(c)), domainMessage.OriginalMediaRequest{
		MessageID: c.Params("message_id"), ChatJID: c.Query("chat_jid"),
		ExpectedDeviceJID: c.Query("expected_device_jid"), ExpectedSHA256: c.Query("expected_sha256"), MimeType: c.Query("mime_type"),
	})
	if err != nil {
		status, code := fiber.StatusServiceUnavailable, "original_media_unavailable"
		switch {
		case errors.Is(err, domainMessage.ErrOriginalMediaInvalid):
			status, code = fiber.StatusUnprocessableEntity, err.Error()
		case errors.Is(err, domainMessage.ErrOriginalMediaScope), errors.Is(err, domainMessage.ErrOriginalMediaIntegrity):
			status, code = fiber.StatusConflict, err.Error()
		case errors.Is(err, domainMessage.ErrOriginalMediaUnavailable):
			status, code = fiber.StatusNotFound, err.Error()
		case errors.Is(err, domainMessage.ErrOriginalMediaTooLarge):
			status, code = fiber.StatusRequestEntityTooLarge, err.Error()
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			status, code = fiber.StatusGatewayTimeout, "original_media_timeout"
		}
		return c.Status(status).JSON(fiber.Map{"code": code})
	}
	c.Set("Cache-Control", "private, no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Disposition", "attachment")
	c.Set("Content-Type", response.MimeType)
	c.Set("X-GoWA-Original-Contract", domainMessage.OriginalMediaContract)
	c.Set("X-GoWA-Device-JID", response.DeviceJID)
	c.Set("X-GoWA-Chat-JID", response.ChatJID)
	c.Set("X-GoWA-Message-ID", response.MessageID)
	c.Set("X-GoWA-Content-SHA256", response.SHA256)
	c.Set("X-GoWA-Media-Type", response.MediaType)
	return c.Send(response.Content)
}
