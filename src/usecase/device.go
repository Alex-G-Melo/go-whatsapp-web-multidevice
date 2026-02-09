package usecase

import (
	"context"
	"fmt"

	domainDevice "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/device"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/websocket"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/validations"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
)

type serviceDevice struct {
	manager *whatsapp.DeviceManager
}

func NewDeviceService(manager *whatsapp.DeviceManager) domainDevice.IDeviceUsecase {
	return &serviceDevice{
		manager: manager,
	}
}

func (s *serviceDevice) ListDevices(_ context.Context) ([]domainDevice.Device, error) {
	if s.manager == nil {
		return []domainDevice.Device{}, nil
	}

	var result []domainDevice.Device
	for _, inst := range s.manager.ListDevices() {
		inst.UpdateStateFromClient()
		result = append(result, convertInstance(inst))
	}
	return result, nil
}

func (s *serviceDevice) GetDevice(_ context.Context, deviceID string) (*domainDevice.Device, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}
	if inst, ok := s.manager.GetDevice(deviceID); ok {
		device := convertInstance(inst)
		return &device, nil
	}
	return nil, fmt.Errorf("device %s not found", deviceID)
}

func (s *serviceDevice) AddDevice(ctx context.Context, deviceID string) (*domainDevice.Device, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}

	inst, err := s.manager.CreateDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	device := convertInstance(inst)
	return &device, nil
}

func (s *serviceDevice) RemoveDevice(_ context.Context, deviceID string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}
	s.manager.RemoveDevice(deviceID)
	return nil
}

func (s *serviceDevice) LoginDevice(_ context.Context, _ string) error {
	return fmt.Errorf("device login per ID is not implemented yet")
}

func (s *serviceDevice) LoginDeviceWithCode(ctx context.Context, deviceID string, phone string) (string, error) {
	if err := validations.ValidateLoginWithCode(ctx, phone); err != nil {
		return "", err
	}

	if s.manager == nil {
		return "", fmt.Errorf("device manager not initialized")
	}

	inst, err := s.manager.EnsureClient(ctx, deviceID)
	if err != nil {
		return "", err
	}

	client := inst.GetClient()
	if client == nil {
		return "", pkgError.ErrWaCLI
	}

	if client.IsLoggedIn() {
		inst.UpdateStateFromClient()
		return "", pkgError.ErrAlreadyLoggedIn
	}

	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return "", err
		}
	}

	logrus.Infof("[LOGIN_CODE][%s] Starting phone pairing for number: %s", deviceID, phone)
	code, err := client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		logrus.Errorf("[LOGIN_CODE][%s] Error when pairing phone: %v", deviceID, err)
		return "", err
	}

	inst.UpdateStateFromClient()
	logrus.Infof("[LOGIN_CODE][%s] Successfully generated pairing code", deviceID)
	return code, nil
}

func (s *serviceDevice) LogoutDevice(ctx context.Context, deviceID string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}

	if err := s.manager.PurgeDevice(ctx, deviceID); err != nil {
		return err
	}

	// Broadcast device removal so UI clients can refresh.
	var devices []domainDevice.Device
	if s.manager != nil {
		for _, inst := range s.manager.ListDevices() {
			inst.UpdateStateFromClient()
			devices = append(devices, convertInstance(inst))
		}
	}

	websocket.Broadcast <- websocket.BroadcastMessage{
		Code:    "DEVICE_REMOVED",
		Message: fmt.Sprintf("Device %s logged out and removed", deviceID),
		Result: map[string]any{
			"device_id": deviceID,
			"devices":   devices,
		},
	}

	return nil
}

func (s *serviceDevice) ReconnectDevice(_ context.Context, deviceID string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}
	if inst, ok := s.manager.GetDevice(deviceID); ok {
		client := inst.GetClient()
		if client == nil {
			return fmt.Errorf("device %s client not initialized", deviceID)
		}

		if client.Store == nil || client.Store.ID == nil {
			return fmt.Errorf("device %s is not logged in (session deleted)", deviceID)
		}

		client.Disconnect()
		return client.Connect()
	}
	return fmt.Errorf("device %s not found", deviceID)
}

func (s *serviceDevice) GetStatus(_ context.Context, deviceID string) (bool, bool, error) {
	if s.manager == nil {
		return false, false, fmt.Errorf("device manager not initialized")
	}
	if inst, ok := s.manager.GetDevice(deviceID); ok {
		inst.UpdateStateFromClient()
		client := inst.GetClient()
		if client == nil {
			return false, false, nil
		}

		if client.Store == nil || client.Store.ID == nil {
			return false, false, nil
		}

		// Update state snapshot based on live client flags
		state := deriveState(inst)
		_ = state
		return client.IsConnected(), client.IsLoggedIn(), nil
	}
	return false, false, fmt.Errorf("device %s not found", deviceID)
}

func convertInstance(inst *whatsapp.DeviceInstance) domainDevice.Device {
	if inst == nil {
		return domainDevice.Device{}
	}

	state := deriveState(inst)

	return domainDevice.Device{
		ID:          inst.ID(),
		PhoneNumber: inst.PhoneNumber(),
		DisplayName: inst.DisplayName(),
		State:       state,
		JID:         inst.JID(),
		CreatedAt:   inst.CreatedAt(),
	}
}

func deriveState(inst *whatsapp.DeviceInstance) domainDevice.DeviceState {
	if inst == nil {
		return domainDevice.DeviceStateDisconnected
	}

	client := inst.GetClient()
	state := inst.State()
	if client != nil {
		if client.IsLoggedIn() {
			state = domainDevice.DeviceStateLoggedIn
		} else if client.IsConnected() {
			state = domainDevice.DeviceStateConnected
		} else {
			state = domainDevice.DeviceStateDisconnected
		}
		inst.SetState(state)
	}

	return state
}
