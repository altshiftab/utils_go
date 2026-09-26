package connect_settings

const (
	BackendTypeSecondGen = "SECOND_GEN"

	IpAddressTypePrimary = "PRIMARY"
	IpAddressTypePrivate = "PRIVATE"
)

type SslCert struct {
	Cert string `json:"cert,omitzero"`
}

type IpMapping struct {
	Type      string `json:"type,omitzero"`
	IpAddress string `json:"ipAddress,omitzero"`
}

type DnsNameMapping struct {
	Name           string `json:"name,omitzero"`
	ConnectionType string `json:"connectionType,omitzero"`
	DnsScope       string `json:"dnsScope,omitzero"`
}

// ConnectSettings is the part of the Cloud SQL Admin API's connectSettings response a dialer needs.
type ConnectSettings struct {
	ServerCaCert    *SslCert          `json:"serverCaCert,omitzero"`
	IpAddresses     []*IpMapping      `json:"ipAddresses,omitzero"`
	Region          string            `json:"region,omitzero"`
	DatabaseVersion string            `json:"databaseVersion,omitzero"`
	BackendType     string            `json:"backendType,omitzero"`
	DnsName         string            `json:"dnsName,omitzero"`
	DnsNames        []*DnsNameMapping `json:"dnsNames,omitzero"`
	ServerCaMode    string            `json:"serverCaMode,omitzero"`
}
