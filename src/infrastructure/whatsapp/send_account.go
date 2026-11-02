package whatsapp

import (
	"context"
	"regexp"

	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"go.mau.fi/whatsmeow"
)

var sendPNJID = regexp.MustCompile(`^[0-9]{1,20}@s\.whatsapp\.net$`)

type sendAccountKey struct{}
type sendAccount struct {
	instance *DeviceInstance
	client   *whatsmeow.Client
	jid      string
}

// ContextWithSendAccount pins the selected client, never a re-resolvable slot alias.
// Empty expectations preserve the existing unbound send contract.
func ContextWithSendAccount(ctx context.Context, client *whatsmeow.Client, expected string) (context.Context, error) {
	if expected == "" {
		return ctx, nil
	}
	if !sendPNJID.MatchString(expected) {
		return ctx, pkgError.ErrSendAccountInvalid
	}
	instance, ok := DeviceFromContext(ctx)
	if !ok || instance == nil {
		return ctx, pkgError.ErrSendAccountMismatch
	}
	ctx = context.WithValue(ctx, sendAccountKey{}, sendAccount{instance, client, expected})
	return ctx, CheckSendAccount(ctx, client)
}

// CheckSendAccount runs again after media preparation and before each physical send.
func CheckSendAccount(ctx context.Context, client *whatsmeow.Client) error {
	binding, ok := ctx.Value(sendAccountKey{}).(sendAccount)
	if !ok {
		return nil
	}
	if client == nil || binding.client != client || binding.instance.GetClient() != client ||
		client.Store == nil || client.Store.ID == nil || client.Store.ID.ToNonAD().String() != binding.jid {
		return pkgError.ErrSendAccountMismatch
	}
	return nil
}

// An SDK success followed by identity drift is ambiguous, never a safe resend.
func CheckSendAccountResult(ctx context.Context, client *whatsmeow.Client) error {
	if err := CheckSendAccount(ctx, client); err != nil {
		return pkgError.ErrSendAccountOutcomeUnknown
	}
	return nil
}
