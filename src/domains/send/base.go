package send

type BaseRequest struct {
	ExpectedDeviceJID string `json:"expected_device_jid,omitempty" form:"expected_device_jid"`
	Phone             string `json:"phone" form:"phone"`
	Duration          *int   `json:"duration,omitempty" form:"duration"`
	IsForwarded       bool   `json:"is_forwarded,omitempty" form:"is_forwarded"`
}
