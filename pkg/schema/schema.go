package schema

import (
	"fmt"
	"net"
	"strconv"

	csp "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/integrity_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/reporting_api"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
)

const unknownPlaceholder = "(unknown)"

type HttpHeaders struct {
	Normalized string `json:"normalized,omitzero"`
	Original   string `json:"original,omitzero"`
}

type Body struct {
	Bytes   int    `json:"bytes,omitzero"`
	Content string `json:"content,omitzero"`
}

type Target struct {
	domain_parts.Parts
	Address string            `json:"address,omitzero"`
	Bytes   int               `json:"bytes,omitzero"`
	Domain  string            `json:"domain,omitzero"`
	Ip      string            `json:"ip,omitzero"`
	Mac     string            `json:"mac,omitzero"`
	Nat     *Nat              `json:"nat,omitzero"`
	Packets int               `json:"packets,omitzero"`
	Port    int               `json:"port,omitzero"`
	As      *AutonomousSystem `json:"as,omitzero"`
	Geo     *Geo              `json:"geo,omitzero"`
	User    *User             `json:"user,omitzero"`
}

type Base struct {
	Timestamp string            `json:"@timestamp,omitzero"`
	Labels    map[string]string `json:"labels,omitzero"`
	Message   string            `json:"message,omitzero"`
	Tags      []string          `json:"tags,omitzero"`

	Client        *Target        `json:"client,omitzero"`
	Cloud         *Cloud         `json:"cloud,omitzero"`
	Container     *Container     `json:"container,omitzero"`
	Destination   *Target        `json:"destination,omitzero"`
	Dns           *Dns           `json:"dns,omitzero"`
	Email         *Email         `json:"email,omitzero"`
	Error         *Error         `json:"error,omitzero"`
	Event         *Event         `json:"event,omitzero"`
	File          *File          `json:"file,omitzero"`
	Group         *Group         `json:"group,omitzero"`
	Host          *Host          `json:"host,omitzero"`
	Http          *Http          `json:"http,omitzero"`
	Log           *Log           `json:"log,omitzero"`
	Observer      *Observer      `json:"observer,omitzero"`
	Package       *Package       `json:"package,omitzero"`
	Process       *Process       `json:"process,omitzero"`
	Registry      *Registry      `json:"registry,omitzero"`
	Related       *Related       `json:"related,omitzero"`
	Rule          *Rule          `json:"rule,omitzero"`
	Server        *Target        `json:"server,omitzero"`
	Service       *Service       `json:"service,omitzero"`
	Source        *Target        `json:"source,omitzero"`
	Threat        *Threat        `json:"threat,omitzero"`
	Tls           *Tls           `json:"tls,omitzero"`
	Network       *Network       `json:"network,omitzero"`
	Url           *Url           `json:"url,omitzero"`
	User          *User          `json:"user,omitzero"`
	UserAgent     *UserAgent     `json:"user_agent,omitzero"`
	Vulnerability *Vulnerability `json:"vulnerability,omitzero"`

	// NOTE: Custom namespaces
	Whois    *Whois    `json:"whois,omitzero"`
	Tcp      *Tcp      `json:"tcp,omitzero"`
	Nftables *Nftables `json:"nftables,omitzero"`
}

func (b *Base) MakeConnectionMessage() string {
	if b == nil {
		return ""
	}

	var sourceIpAddress string
	var destinationIpAddress string
	var sourcePort int
	var destinationPort int

	if ecsSource := b.Source; ecsSource != nil {
		sourceIpAddress = ecsSource.Ip
		sourcePort = ecsSource.Port
	}

	if ecsDestination := b.Destination; ecsDestination != nil {
		destinationIpAddress = ecsDestination.Ip
		destinationPort = ecsDestination.Port
	}

	sourcePart := unknownPlaceholder
	if sourceIpAddress != "" && sourcePort != 0 {
		sourcePart = net.JoinHostPort(sourceIpAddress, strconv.Itoa(sourcePort))
	} else if sourceIpAddress != "" {
		sourcePart = sourceIpAddress
	} else if sourcePort != 0 {
		sourcePart = fmt.Sprintf(":%d", sourcePort)
	}

	destinationPart := unknownPlaceholder
	if destinationIpAddress != "" && destinationPort != 0 {
		destinationPart = net.JoinHostPort(destinationIpAddress, strconv.Itoa(destinationPort))
	} else if destinationIpAddress != "" {
		destinationPart = destinationIpAddress
	} else if destinationPort != 0 {
		destinationPart = fmt.Sprintf(":%d", destinationPort)
	}

	transportPart := unknownPlaceholder
	if ecsNetwork := b.Network; ecsNetwork != nil {
		if ecsNetwork.Transport != "" {
			transportPart = ecsNetwork.Transport
		} else if ecsNetwork.IanaNumber != "" {
			transportPart = fmt.Sprintf("(%s)", ecsNetwork.IanaNumber)
		}
	}

	var message string

	if sourcePart == unknownPlaceholder && destinationPart == unknownPlaceholder && transportPart == unknownPlaceholder {
		message = unknownPlaceholder
	} else {
		message = fmt.Sprintf("%s -> %s %s", sourcePart, destinationPart, transportPart)
	}

	return message
}

type AgentBuild struct {
	Original string `json:"original,omitzero"`
}

type Agent struct {
	Build       *AgentBuild `json:"build,omitzero"`
	EphemeralId string      `json:"ephemeral_id,omitzero"`
	Id          string      `json:"id,omitzero"`
	Name        string      `json:"name,omitzero"`
	Type        string      `json:"type,omitzero"`
	Version     string      `json:"version,omitzero"`
}

type AutonomousSystem struct {
	Number       int64         `json:"number,omitzero"`
	Organization *Organization `json:"organization,omitzero"`
}

type Geo struct {
	CityName       string `json:"city_name,omitzero"`
	ContinentCode  string `json:"continent_code,omitzero"`
	ContinentName  string `json:"continent_name,omitzero"`
	CountryIsoCode string `json:"country_iso_code,omitzero"`
	CountryName    string `json:"country_name,omitzero"`
	Location       any    `json:"location,omitzero"`
	Name           string `json:"name,omitzero"`
	PostalCode     string `json:"postal_code,omitzero"`
	RegionIsoCode  string `json:"region_iso_code,omitzero"`
	RegionName     string `json:"region_name,omitzero"`
	Timezone       string `json:"timezone,omitzero"`
}

type Nat struct {
	Ip   string `json:"ip,omitzero"`
	Port int    `json:"port,omitzero"`
}

type Group struct {
	Domain string `json:"domain,omitzero"`
	Id     string `json:"id,omitzero"`
	Name   string `json:"name,omitzero"`
}

type User struct {
	Domain   string   `json:"domain,omitzero"`
	Email    string   `json:"email,omitzero"`
	FullName string   `json:"full_name,omitzero"`
	Hash     string   `json:"hash,omitzero"`
	Id       string   `json:"id,omitzero"`
	Name     string   `json:"name,omitzero"`
	Roles    []string `json:"roles,omitzero"`
	Changes  *User    `json:"changes,omitzero"`
	Group    *Group   `json:"group,omitzero"`
	Target   *User    `json:"target,omitzero"`
	// NOTE: Custom
	Unverified bool `json:"unverified,omitzero"`
}

type CloudAccount struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type CloudInstance struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type CloudMachine struct {
	Type string `json:"type,omitzero"`
}

type CloudProject struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type CloudService struct {
	Name string `json:"name,omitzero"`
}

type CloudOriginTargetAccount struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type CloudOriginTargetInstance struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type CloudOriginTargetMachine struct {
	Type string `json:"type,omitzero"`
}

type CloudOriginTargetProject struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type CloudOriginTargetService struct {
	Name string `json:"name,omitzero"`
}

type CloudOriginTarget struct {
	Account          *CloudOriginTargetAccount  `json:"account,omitzero"`
	AvailabilityZone string                     `json:"availability_zone,omitzero"`
	Instance         *CloudOriginTargetInstance `json:"instance,omitzero"`
	Machine          *CloudOriginTargetMachine  `json:"machine,omitzero"`
	Project          *CloudOriginTargetProject  `json:"project,omitzero"`
	Provider         string                     `json:"provider,omitzero"`
	Region           string                     `json:"region,omitzero"`
	Service          *CloudOriginTargetService  `json:"service,omitzero"`
}

type Cloud struct {
	Account          *CloudAccount      `json:"account,omitzero"`
	AvailabilityZone string             `json:"availability_zone,omitzero"`
	Instance         *CloudInstance     `json:"instance,omitzero"`
	Machine          *CloudMachine      `json:"machine,omitzero"`
	Origin           *CloudOriginTarget `json:"origin,omitzero"`
	Project          *CloudProject      `json:"project,omitzero"`
	Provider         string             `json:"provider,omitzero"`
	Region           string             `json:"region,omitzero"`
	Service          *CloudService      `json:"service,omitzero"`
	Target           *CloudOriginTarget `json:"target,omitzero"`
}

type ContainerImageHash struct {
	All []string `json:"all,omitzero"`
}

type ContainerImage struct {
	Hash *ContainerImageHash `json:"hash,omitzero"`
	Name string              `json:"name,omitzero"`
	Tag  string              `json:"tag,omitzero"`
}

type Container struct {
	Id      string          `json:"id,omitzero"`
	Image   *ContainerImage `json:"image,omitzero"`
	Labels  any             `json:"labels,omitzero"`
	Name    string          `json:"name,omitzero"`
	Runtime string          `json:"runtime,omitzero"`
}

type DnsAnswer struct {
	Class string `json:"class,omitzero"`
	Data  string `json:"data,omitzero"`
	Name  string `json:"name,omitzero"`
	Ttl   int    `json:"ttl,omitzero"`
	Type  string `json:"type,omitzero"`
}

type DnsQuestion struct {
	domain_parts.Parts
	Class string `json:"class,omitzero"`
	Name  string `json:"name,omitzero"`
	Type  string `json:"type,omitzero"`
}

type Dns struct {
	Answers      []*DnsAnswer `json:"answers,omitzero"`
	HeaderFlags  []string     `json:"header_flags,omitzero"`
	Id           string       `json:"id,omitzero"`
	OpCode       string       `json:"op_code,omitzero"`
	Question     *DnsQuestion `json:"question,omitzero"`
	ResolvedIp   []string     `json:"resolved_ip,omitzero"`
	ResponseCode string       `json:"response_code,omitzero"`
	Type         string       `json:"type,omitzero"`
}

type Error struct {
	// NOTE: Custom
	Cause      *Error `json:"cause,omitzero"`
	Code       string `json:"code,omitzero"`
	Id         string `json:"id,omitzero"`
	Message    string `json:"message,omitzero"`
	StackTrace string `json:"stack_trace,omitzero"`
	Type       string `json:"type,omitzero"`
}

type EmailAttachmentFile struct {
	Extension string `json:"extension,omitzero"`
	MimeType  string `json:"mime_type,omitzero"`
	Name      string `json:"name,omitzero"`
	Size      int    `json:"size,omitzero"`
	Hash      *Hash  `json:"hash,omitzero"`
}

type EmailAttachment struct {
	File *EmailAttachmentFile `json:"file,omitzero"`
}

type EmailAddress struct {
	// NOTE: Custom
	Name    string `json:"name,omitzero"`
	Address string `json:"address,omitzero"`
}

type Email struct {
	Attachments          []*EmailAttachment `json:"attachments,omitzero"`
	Bcc                  []*EmailAddress    `json:"bcc,omitzero"`
	Cc                   []*EmailAddress    `json:"cc,omitzero"`
	ContentType          string             `json:"content_type,omitzero"`
	DeliveryTimestamp    string             `json:"delivery_timestamp,omitzero"`
	Direction            string             `json:"direction,omitzero"`
	From                 []*EmailAddress    `json:"from,omitzero"`
	LocalId              string             `json:"local_id,omitzero"`
	MessageId            string             `json:"message_id,omitzero"`
	OriginationTimestamp string             `json:"origination_timestamp,omitzero"`
	ReplyTo              []*EmailAddress    `json:"reply_to,omitzero"`
	Sender               *EmailAddress      `json:"sender,omitzero"`
	Subject              string             `json:"subject,omitzero"`
	To                   []*EmailAddress    `json:"to,omitzero"`
	XMailer              string             `json:"x_mailer,omitzero"`
}

type Event struct {
	Action        string   `json:"action,omitzero"`
	AgentIdStatus string   `json:"agent_id_status,omitzero"`
	Category      []string `json:"category,omitzero"`
	Code          string   `json:"code,omitzero"`
	Created       string   `json:"created,omitzero"`
	Dataset       string   `json:"dataset,omitzero"`
	Duration      int64    `json:"duration,omitzero"`
	End           string   `json:"end,omitzero"`
	Hash          string   `json:"hash,omitzero"`
	Id            string   `json:"id,omitzero"`
	Ingested      string   `json:"ingested,omitzero"`
	Kind          string   `json:"kind,omitzero"`
	Module        string   `json:"module,omitzero"`
	Original      string   `json:"original,omitzero"`
	Outcome       string   `json:"outcome,omitzero"`
	Provider      string   `json:"provider,omitzero"`
	Reason        string   `json:"reason,omitzero"`
	Reference     string   `json:"reference,omitzero"`
	RiskScore     float64  `json:"risk_score,omitzero"`
	Sequence      int      `json:"sequence,omitzero"`
	Severity      int      `json:"severity,omitzero"`
	Start         string   `json:"start,omitzero"`
	Timezone      string   `json:"timezone,omitzero"`
	Type          []string `json:"type,omitzero"`
	Url           string   `json:"url,omitzero"`
}

type Hash struct {
	Cdhash string `json:"cdhash,omitzero"`
	Md5    string `json:"md5,omitzero"`
	Sha1   string `json:"sha1,omitzero"`
	Sha256 string `json:"sha256,omitzero"`
	Sha384 string `json:"sha384,omitzero"`
	Sha512 string `json:"sha512,omitzero"`
	Ssdeep string `json:"ssdeep,omitzero"`
	Tlsh   string `json:"tlsh,omitzero"`
}

type File struct {
	Accessed    string   `json:"accessed,omitzero"`
	Attributes  []string `json:"attributes,omitzero"`
	Created     string   `json:"created,omitzero"`
	Ctime       string   `json:"ctime,omitzero"`
	Device      string   `json:"device,omitzero"`
	Directory   string   `json:"directory,omitzero"`
	DriveLetter string   `json:"drive_letter,omitzero"`
	Extension   string   `json:"extension,omitzero"`
	Gid         string   `json:"gid,omitzero"`
	Group       string   `json:"group,omitzero"`
	Hash        *Hash    `json:"hash,omitzero"`
	Inode       string   `json:"inode,omitzero"`
	Mode        string   `json:"mode,omitzero"`
	Mtime       string   `json:"mtime,omitzero"`
	Name        string   `json:"name,omitzero"`
	Owner       string   `json:"owner,omitzero"`
	Path        string   `json:"path,omitzero"`
	Size        int64    `json:"size,omitzero"`
	Type        string   `json:"type,omitzero"`
	Uid         string   `json:"uid,omitzero"`
}

type Host struct {
	Architecture string   `json:"architecture,omitzero"`
	Domain       string   `json:"domain,omitzero"`
	Hostname     string   `json:"hostname,omitzero"`
	Id           string   `json:"id,omitzero"`
	Ip           []string `json:"ip,omitzero"`
	Mac          []string `json:"mac,omitzero"`
	Name         string   `json:"name,omitzero"`
	Type         string   `json:"type,omitzero"`
	Uptime       int64    `json:"uptime,omitzero"`
	Os           Os       `json:"os,omitzero"`
}

type HttpReporting struct {
	IntegrityViolations []*reporting_api.Report[*integrity_policy.IntegrityViolationReportBody] `json:"integrity_violations,omitzero"`
	CspViolations       []*reporting_api.Report[*csp.CSPViolationReportBody]                    `json:"csp_violations,omitzero"`
	CspReport           *csp.ReportEnvelope                                                     `json:"csp_report,omitzero"`
}

type HttpRequest struct {
	Body  *Body `json:"body,omitzero"`
	Bytes int   `json:"bytes,omitzero"`
	// NOTE: Custom
	ContentType string `json:"content_type,omitzero"`
	// NOTE: Custom
	HttpHeaders *HttpHeaders `json:"headers,omitzero"`
	Id          string       `json:"id,omitzero"`
	Method      string       `json:"method,omitzero"`
	MimeType    string       `json:"mime_type,omitzero"`
	Referrer    string       `json:"referrer,omitzero"`
	// NOTE: Custom
	Reporting *HttpReporting `json:"reporting,omitzero"`
}

type HttpResponse struct {
	Body  *Body `json:"body,omitzero"`
	Bytes int   `json:"bytes,omitzero"`
	// NOTE: Custom
	ContentType string `json:"content_type,omitzero"`
	// NOTE: Custom
	HttpHeaders *HttpHeaders `json:"headers,omitzero"`
	MimeType    string       `json:"mime_type,omitzero"`
	// NOTE: Custom
	ReasonPhrase string `json:"reason_phrase,omitzero"`
	StatusCode   int    `json:"status_code,omitzero"`
	// TODO: Add parsed problem detail?
}

type Http struct {
	Request  *HttpRequest  `json:"request,omitzero"`
	Response *HttpResponse `json:"response,omitzero"`
	Version  string        `json:"version,omitzero"`
}

type Interface struct {
	Alias string `json:"alias,omitzero"`
	Id    string `json:"id,omitzero"`
	Name  string `json:"name,omitzero"`
}

type Network struct {
	Application string `json:"application,omitzero"`
	Bytes       int64  `json:"bytes,omitzero"`
	// NOTE: Made into an array to be able to capture both Source-Destination and Client-Server, which can differ.
	CommunityId []string `json:"community_id,omitzero"`
	Direction   string   `json:"direction,omitzero"`
	ForwardedIp string   `json:"forwarded_ip,omitzero"`
	IanaNumber  string   `json:"iana_number,omitzero"`
	Inner       any      `json:"inner,omitzero"`
	Name        string   `json:"name,omitzero"`
	Packets     int64    `json:"packets,omitzero"`
	Protocol    string   `json:"protocol,omitzero"`
	Transport   string   `json:"transport,omitzero"`
	Type        string   `json:"type,omitzero"`
}

type LogOriginFile struct {
	Line int    `json:"line,omitzero"`
	Name string `json:"name,omitzero"`
}

type LogFile struct {
	Path string `json:"path,omitzero"`
}

type LogOrigin struct {
	File     *LogOriginFile `json:"file,omitzero"`
	Function string         `json:"function,omitzero"`
	// NOTE: Custom
	Process *Process `json:"process,omitzero"`
}

type LogSyslogFacility struct {
	Code int    `json:"code,omitzero"`
	Name string `json:"name,omitzero"`
}

type LogSyslogSeverity struct {
	Code int    `json:"code,omitzero"`
	Name string `json:"name,omitzero"`
}

type LogSyslog struct {
	Appname        string             `json:"appname,omitzero"`
	Facility       *LogSyslogFacility `json:"facility,omitzero"`
	Hostname       string             `json:"hostname,omitzero"`
	Msgid          string             `json:"msgid,omitzero"`
	Priority       int                `json:"priority,omitzero"`
	Procid         string             `json:"procid,omitzero"`
	StructuredData map[string]any     `json:"structured_data,omitzero"`
	Version        string             `json:"version,omitzero"`
}

type Log struct {
	LogFile *LogFile   `json:"file,omitzero"`
	Level   string     `json:"level,omitzero"`
	Logger  string     `json:"logger,omitzero"`
	Origin  *LogOrigin `json:"origin,omitzero"`
	Syslog  *LogSyslog `json:"syslog,omitzero"`
}

type ObserverIngressEgress struct {
	Interface *Interface `json:"interface,omitzero"`
	Zone      string     `json:"zone,omitzero"`
}

type Observer struct {
	Egress       *ObserverIngressEgress `json:"egress,omitzero"`
	Hostname     string                 `json:"hostname,omitzero"`
	Ingress      *ObserverIngressEgress `json:"ingress,omitzero"`
	Ip           string                 `json:"ip,omitzero"`
	Mac          string                 `json:"mac,omitzero"`
	Name         string                 `json:"name,omitzero"`
	Os           *Os                    `json:"os,omitzero"`
	Product      string                 `json:"product,omitzero"`
	SerialNumber string                 `json:"serial_number,omitzero"`
	Type         string                 `json:"type,omitzero"`
	Vendor       string                 `json:"vendor,omitzero"`
	Version      string                 `json:"version,omitzero"`
}

type Organization struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type Os struct {
	Family   string `json:"family,omitzero"`
	Full     string `json:"full,omitzero"`
	Kernel   string `json:"kernel,omitzero"`
	Name     string `json:"name,omitzero"`
	Platform string `json:"platform,omitzero"`
	Type     string `json:"type,omitzero"`
	Version  string `json:"version,omitzero"`
}

type Package struct {
	Architecture string `json:"architecture,omitzero"`
	BuildVersion string `json:"build_version,omitzero"`
	Checksum     string `json:"checksum,omitzero"`
	Description  string `json:"description,omitzero"`
	InstallScope string `json:"install_scope,omitzero"`
	Installed    string `json:"installed,omitzero"`
	Licence      string `json:"licence,omitzero"`
	Name         string `json:"name,omitzero"`
	Path         string `json:"path,omitzero"`
	Reference    string `json:"reference,omitzero"`
	Size         string `json:"size,omitzero"`
	Type         string `json:"type,omitzero"`
	Version      string `json:"version,omitzero"`
}

type ProcessIo struct {
	Text string `json:"text,omitzero"`
}

type ProcessThreadCapabilities struct {
	Effective []string `json:"effective,omitzero"`
	Permitted []string `json:"permitted,omitzero"`
}

type ProcessThread struct {
	Id   int    `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type Process struct {
	Args             []string       `json:"args,omitzero"`
	ArgsCount        int            `json:"args_count,omitzero"`
	CommandLine      string         `json:"command_line,omitzero"`
	End              string         `json:"end,omitzero"`
	EnvVars          []string       `json:"env_vars,omitzero"`
	Executable       string         `json:"executable,omitzero"`
	ExitCode         int            `json:"exit_code,omitzero"`
	Group            *Group         `json:"group,omitzero"`
	Interactive      bool           `json:"interactive,omitzero"`
	Io               *ProcessIo     `json:"io,omitzero"`
	Name             string         `json:"name,omitzero"`
	Parent           *Process       `json:"parent,omitzero"`
	Pid              int            `json:"pid,omitzero"`
	Previous         *Process       `json:"previous,omitzero"`
	Start            string         `json:"start,omitzero"`
	Thread           *ProcessThread `json:"thread,omitzero"`
	Title            string         `json:"title,omitzero"`
	Uptime           int            `json:"uptime,omitzero"`
	User             *User          `json:"user,omitzero"`
	WorkingDirectory string         `json:"working_directory,omitzero"`
}

type RegistryData struct {
	Bytes   string   `json:"bytes,omitzero"`
	Strings []string `json:"strings,omitzero"`
	Type    string   `json:"type,omitzero"`
}

type Registry struct {
	Data  *RegistryData `json:"data,omitzero"`
	Hive  string        `json:"hive,omitzero"`
	Key   string        `json:"key,omitzero"`
	Path  string        `json:"path,omitzero"`
	Value string        `json:"value,omitzero"`
}

type Related struct {
	Hash  []string `json:"hash,omitzero"`
	Hosts []string `json:"hosts,omitzero"`
	Ip    []string `json:"ip,omitzero"`
	User  []string `json:"user,omitzero"`
}

type Rule struct {
	Author      string `json:"author,omitzero"`
	Category    string `json:"category,omitzero"`
	Description string `json:"description,omitzero"`
	Id          string `json:"id,omitzero"`
	License     string `json:"license,omitzero"`
	Name        string `json:"name,omitzero"`
	Reference   string `json:"reference,omitzero"`
	Ruleset     string `json:"ruleset,omitzero"`
	UUID        string `json:"uuid,omitzero"`
	Version     string `json:"version,omitzero"`
}

type ServiceNode struct {
	Name  string   `json:"name,omitzero"`
	Role  string   `json:"role,omitzero"`
	Roles []string `json:"roles,omitzero"`
}

type Service struct {
	Address     string       `json:"address,omitzero"`
	Environment string       `json:"environment,omitzero"`
	EphemeralId string       `json:"ephemeral_id,omitzero"`
	Id          string       `json:"id,omitzero"`
	Name        string       `json:"name,omitzero"`
	Node        *ServiceNode `json:"node,omitzero"`
	State       string       `json:"state,omitzero"`
	Type        string       `json:"type,omitzero"`
	Version     string       `json:"version,omitzero"`

	Origin *Service `json:"origin,omitzero"`
	Target *Service `json:"target,omitzero"`
}

// NOTE: Custom

type Tcp struct {
	Flags                 []string `json:"flags,omitzero"`
	AcknowledgementNumber *int     `json:"acknowledgement_number,omitzero"`
	SequenceNumber        *int     `json:"sequence_number,omitzero"`
	State                 string   `json:"state,omitzero"`
}

type Nftables struct {
	ConntrackState string  `json:"conntrack_state,omitzero"`
	Hook           string  `json:"hook,omitzero"`
	HwAddr         string  `json:"hw_addr,omitzero"`
	Mark           *uint32 `json:"mark,omitzero"`
}

type ThreatGroup struct {
	Alias     []string `json:"alias,omitzero"`
	Id        string   `json:"id,omitzero"`
	Name      string   `json:"name,omitzero"`
	Reference string   `json:"reference,omitzero"`
}

type ThreatFeed struct {
	Name      string `json:"name,omitzero"`
	Reference string `json:"reference,omitzero"`
}

type ThreatTechnique struct {
	Id           []string         `json:"id,omitzero"`
	Name         []string         `json:"name,omitzero"`
	Reference    []string         `json:"reference,omitzero"`
	Subtechnique *ThreatTechnique `json:"subtechnique,omitzero"`
}

type ThreatTactic struct {
	Id        []string `json:"id,omitzero"`
	Name      []string `json:"name,omitzero"`
	Reference []string `json:"reference,omitzero"`
}

type ThreatSoftware struct {
	Alias     string   `json:"alias,omitzero"`
	Id        string   `json:"id,omitzero"`
	Name      string   `json:"name,omitzero"`
	Platforms []string `json:"platforms,omitzero"`
	Reference string   `json:"reference,omitzero"`
	Type      string   `json:"type,omitzero"`
}

type ThreatIndicatorMarking struct {
	Tlp        string `json:"tlp,omitzero"`
	TlpVersion string `json:"tlp_version,omitzero"`
}

type ThreatIndicatorEmail struct {
	Address string `json:"address,omitzero"`
}

type ThreatIndicator struct {
	Confidence   string                  `json:"confidence,omitzero"`
	Description  string                  `json:"description,omitzero"`
	Email        *ThreatIndicatorEmail   `json:"email,omitzero"`
	File         *File                   `json:"file,omitzero"`
	Geo          *Geo                    `json:"geo,omitzero"`
	FirstSeen    string                  `json:"first_seen,omitzero"`
	Id           string                  `json:"id,omitzero"`
	Ip           string                  `json:"ip,omitzero"`
	LastSeen     string                  `json:"last_seen,omitzero"`
	Marking      *ThreatIndicatorMarking `json:"marking,omitzero"`
	ModifiedAt   string                  `json:"modified_at,omitzero"`
	Name         string                  `json:"name,omitzero"`
	Port         *int                    `json:"port,omitzero"`
	Provider     string                  `json:"provider,omitzero"`
	Reference    string                  `json:"reference,omitzero"`
	Registry     *Registry               `json:"registry,omitzero"`
	ScannerStats *int                    `json:"scanner_stats,omitzero"`
	Sightings    *int                    `json:"sightings,omitzero"`
	Type         string                  `json:"type,omitzero"`
	Url          *Url                    `json:"url,omitzero"`
}

type ThreatEnrichmentMatched struct {
	Atomic   string `json:"atomic,omitzero"`
	Field    string `json:"field,omitzero"`
	Id       string `json:"id,omitzero"`
	Index    string `json:"index,omitzero"`
	Occurred string `json:"occurred,omitzero"`
	Type     string `json:"type,omitzero"`
}

type ThreatEnrichment struct {
	Indicator *ThreatIndicator         `json:"indicator,omitzero"`
	Matched   *ThreatEnrichmentMatched `json:"matched,omitzero"`
}

type Threat struct {
	Enrichments []*ThreatEnrichment `json:"enrichments,omitzero"`
	Feed        *ThreatFeed         `json:"feed,omitzero"`
	Framework   string              `json:"framework,omitzero"`
	Group       *ThreatGroup        `json:"group,omitzero"`
	Indicator   *ThreatIndicator    `json:"indicator,omitzero"`
	Software    *ThreatSoftware     `json:"software,omitzero"`
	Tactic      *ThreatTactic       `json:"tactic,omitzero"`
	Technique   *ThreatTechnique    `json:"technique,omitzero"`
}

type TlsHash struct {
	Md5    string `json:"md5,omitzero"`
	Sha1   string `json:"sha1,omitzero"`
	Sha256 string `json:"sha256,omitzero"`
}

type TlsClient struct {
	Certificate      string   `json:"certificate,omitzero"`
	CertificateChain []string `json:"certificate_chain,omitzero"`
	Hash             *TlsHash `json:"hash,omitzero"`
	Issuer           string   `json:"issuer,omitzero"`
	Ja3              string   `json:"ja3,omitzero"`
	Ja4              string   `json:"ja4,omitzero"`
	NotAfter         string   `json:"not_after,omitzero"`
	NotBefore        string   `json:"not_before,omitzero"`
	ServerName       string   `json:"server_name,omitzero"`
	Subject          string   `json:"subject,omitzero"`
	SupportedCiphers []string `json:"supported_ciphers,omitzero"`
	X509             *X509    `json:"x509,omitzero"`
}

type TlsServer struct {
	Certificate      string   `json:"certificate,omitzero"`
	CertificateChain []string `json:"certificate_chain,omitzero"`
	Hash             *TlsHash `json:"hash,omitzero"`
	Issuer           string   `json:"issuer,omitzero"`
	Ja3s             string   `json:"ja3s,omitzero"`
	NotAfter         string   `json:"not_after,omitzero"`
	NotBefore        string   `json:"not_before,omitzero"`
	Subject          string   `json:"subject,omitzero"`
	X509             *X509    `json:"x509,omitzero"`
}

// NOTE: Custom/OpenTelemetry

type TlsProtocol struct {
	Name    string `json:"name,omitzero"`
	Version string `json:"version,omitzero"`
}

type Tls struct {
	Cipher          string       `json:"cipher,omitzero"`
	Client          *TlsClient   `json:"client,omitzero"`
	Curve           string       `json:"curve,omitzero"`
	Established     bool         `json:"established,omitzero"`
	NextProtocol    string       `json:"next_protocol,omitzero"`
	Resumed         bool         `json:"resumed,omitzero"`
	Server          *TlsServer   `json:"server,omitzero"`
	TlsProtocol     *TlsProtocol `json:"protocol,omitzero"`
	Version         string       `json:"version,omitzero"`
	VersionProtocol string       `json:"version_protocol,omitzero"`
}

type Url struct {
	domain_parts.Parts
	Domain    string `json:"domain,omitzero"`
	Extension string `json:"extension,omitzero"`
	Fragment  string `json:"fragment,omitzero"`
	Full      string `json:"full,omitzero"`
	Original  string `json:"original,omitzero"`
	Password  string `json:"password,omitzero"`
	Path      string `json:"path,omitzero"`
	Port      int    `json:"port,omitzero"`
	Query     string `json:"query,omitzero"`
	Scheme    string `json:"scheme,omitzero"`
	Username  string `json:"username,omitzero"`
}

type UserAgentDevice struct {
	Name string `json:"name,omitzero"`
}

type UserAgent struct {
	Device   *UserAgentDevice `json:"device,omitzero"`
	Name     string           `json:"name,omitzero"`
	Original string           `json:"original,omitzero"`
	Os       *Os              `json:"os,omitzero"`
	Version  string           `json:"version,omitzero"`
}

type VulnerabilityScanner struct {
	Vendor string `json:"vendor,omitzero"`
}

type VulnerabilityScore struct {
	Base          float64 `json:"base,omitzero"`
	Environmental float64 `json:"environmental,omitzero"`
	Temporal      float64 `json:"temporal,omitzero"`
	Version       string  `json:"version,omitzero"`
}

type Vulnerability struct {
	Category       string                `json:"category,omitzero"`
	Classification string                `json:"classification,omitzero"`
	Description    string                `json:"description,omitzero"`
	Enumeration    string                `json:"enumeration,omitzero"`
	Id             string                `json:"id,omitzero"`
	Reference      string                `json:"reference,omitzero"`
	ReportId       string                `json:"report_id,omitzero"`
	Scanner        *VulnerabilityScanner `json:"scanner,omitzero"`
	Score          *VulnerabilityScore   `json:"score,omitzero"`
	Severity       string                `json:"severity,omitzero"`
}

type X509Target struct {
	CommonName         []string `json:"common_name,omitzero"`
	Country            []string `json:"country,omitzero"`
	DistinguishedName  string   `json:"distinguished_name,omitzero"`
	Locality           []string `json:"locality,omitzero"`
	Organization       []string `json:"organization,omitzero"`
	OrganizationalUnit []string `json:"organizational_unit,omitzero"`
	StateOrProvince    []string `json:"state_or_province,omitzero"`
}

type X509 struct {
	AlternativeNames   []string    `json:"alternate_names,omitzero"`
	Issuer             *X509Target `json:"issuer,omitzero"`
	NotAfter           string      `json:"not_after,omitzero"`
	NotBefore          string      `json:"not_before,omitzero"`
	PublicKeyAlgorithm string      `json:"public_key_algorithm,omitzero"`
	PublicKeyCurve     string      `json:"public_key_curve,omitzero"`
	PublicKeyExponent  int         `json:"public_key_exponent,omitzero"`
	PublicKeySize      int         `json:"public_key_size,omitzero"`
	SerialNumber       string      `json:"serial_number,omitzero"`
	SignatureAlgorithm string      `json:"signature_algorithm,omitzero"`
	Subject            *X509Target `json:"subject,omitzero"`
	VersionNumber      string      `json:"version_number,omitzero"`
}

// NOTE: Custom

type WhoisRequest struct {
	Body *Body  `json:"body,omitzero"`
	Id   string `json:"id,omitzero"`
}

// NOTE: Custom

type WhoisResponse struct {
	Body *Body `json:"body,omitzero"`
}

// NOTE: Custom

type Whois struct {
	Request  *WhoisRequest  `json:"request,omitzero"`
	Response *WhoisResponse `json:"response,omitzero"`
}
