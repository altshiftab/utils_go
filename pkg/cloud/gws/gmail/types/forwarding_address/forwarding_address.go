package forwarding_address

// VerificationStatus is whether Gmail may forward to the address yet.
type VerificationStatus string

const (
	VerificationStatusAccepted VerificationStatus = "accepted"
	VerificationStatusPending  VerificationStatus = "pending"
)

type ForwardingAddress struct {
	ForwardingEmail    string             `json:"forwardingEmail,omitzero"`
	VerificationStatus VerificationStatus `json:"verificationStatus,omitzero"`
}
