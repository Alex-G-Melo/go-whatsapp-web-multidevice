package send

const PhysicalSendContract = "gowa-send-account-1"

type GenericResponse struct {
	DeviceJID string `json:"device_jid,omitempty"`
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}
