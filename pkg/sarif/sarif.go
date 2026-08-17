package sarif

const (
	Version       = "2.1.0"
	SchemaUri     = "https://json.schemastore.org/sarif-2.1.0.json"
	BomFormatName = "SARIF"
)

type Level string

const (
	LevelNone    Level = "none"
	LevelNote    Level = "note"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

type Kind string

const (
	KindNotApplicable Kind = "notApplicable"
	KindPass          Kind = "pass"
	KindFail          Kind = "fail"
	KindReview        Kind = "review"
	KindOpen          Kind = "open"
	KindInformational Kind = "informational"
)

type BaselineState string

const (
	BaselineStateNew       BaselineState = "new"
	BaselineStateUnchanged BaselineState = "unchanged"
	BaselineStateUpdated   BaselineState = "updated"
	BaselineStateAbsent    BaselineState = "absent"
)

type Importance string

const (
	ImportanceImportant   Importance = "important"
	ImportanceEssential   Importance = "essential"
	ImportanceUnimportant Importance = "unimportant"
)

type SuppressionKind string

const (
	SuppressionKindInSource SuppressionKind = "inSource"
	SuppressionKindExternal SuppressionKind = "external"
)

type SuppressionStatus string

const (
	SuppressionStatusAccepted    SuppressionStatus = "accepted"
	SuppressionStatusUnderReview SuppressionStatus = "underReview"
	SuppressionStatusRejected    SuppressionStatus = "rejected"
)

type ColumnKind string

const (
	ColumnKindUtf16CodeUnits    ColumnKind = "utf16CodeUnits"
	ColumnKindUnicodeCodePoints ColumnKind = "unicodeCodePoints"
)

type Role string

const (
	RoleAnalysisTarget             Role = "analysisTarget"
	RoleAttachment                 Role = "attachment"
	RoleResponseFile               Role = "responseFile"
	RoleResultFile                 Role = "resultFile"
	RoleStandardStream             Role = "standardStream"
	RoleTracedFile                 Role = "tracedFile"
	RoleUnmodified                 Role = "unmodified"
	RoleModified                   Role = "modified"
	RoleAdded                      Role = "added"
	RoleDeleted                    Role = "deleted"
	RoleRenamed                    Role = "renamed"
	RoleUncontrolled               Role = "uncontrolled"
	RoleDriver                     Role = "driver"
	RoleExtension                  Role = "extension"
	RoleTranslation                Role = "translation"
	RoleTaxonomy                   Role = "taxonomy"
	RolePolicy                     Role = "policy"
	RoleReferencedOnCommandLine    Role = "referencedOnCommandLine"
	RoleMemoryContents             Role = "memoryContents"
	RoleDirectory                  Role = "directory"
	RoleUserSpecifiedConfiguration Role = "userSpecifiedConfiguration"
	RoleToolSpecifiedConfiguration Role = "toolSpecifiedConfiguration"
	RoleDebugOutputFile            Role = "debugOutputFile"
)

type PropertyBag map[string]any

type MultiformatMessageString struct {
	Text       string      `json:"text"`
	Markdown   string      `json:"markdown,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type Message struct {
	Text       string      `json:"text,omitzero"`
	Markdown   string      `json:"markdown,omitzero"`
	Id         string      `json:"id,omitzero"`
	Arguments  []string    `json:"arguments,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type ArtifactLocation struct {
	Uri         string      `json:"uri,omitzero"`
	UriBaseId   string      `json:"uriBaseId,omitzero"`
	Index       *int        `json:"index,omitzero"`
	Description *Message    `json:"description,omitzero"`
	Properties  PropertyBag `json:"properties,omitzero"`
}

type ArtifactContent struct {
	Text       string                    `json:"text,omitzero"`
	Binary     string                    `json:"binary,omitzero"`
	Rendered   *MultiformatMessageString `json:"rendered,omitzero"`
	Properties PropertyBag               `json:"properties,omitzero"`
}

type Region struct {
	StartLine      int              `json:"startLine,omitzero"`
	StartColumn    int              `json:"startColumn,omitzero"`
	EndLine        int              `json:"endLine,omitzero"`
	EndColumn      int              `json:"endColumn,omitzero"`
	CharOffset     int              `json:"charOffset,omitzero"`
	CharLength     int              `json:"charLength,omitzero"`
	ByteOffset     int              `json:"byteOffset,omitzero"`
	ByteLength     int              `json:"byteLength,omitzero"`
	Snippet        *ArtifactContent `json:"snippet,omitzero"`
	Message        *Message         `json:"message,omitzero"`
	SourceLanguage string           `json:"sourceLanguage,omitzero"`
	Properties     PropertyBag      `json:"properties,omitzero"`
}

type Rectangle struct {
	Top        float64     `json:"top,omitzero"`
	Left       float64     `json:"left,omitzero"`
	Bottom     float64     `json:"bottom,omitzero"`
	Right      float64     `json:"right,omitzero"`
	Message    *Message    `json:"message,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type Address struct {
	AbsoluteAddress    int         `json:"absoluteAddress,omitzero"`
	RelativeAddress    int         `json:"relativeAddress,omitzero"`
	Length             int         `json:"length,omitzero"`
	Kind               string      `json:"kind,omitzero"`
	Name               string      `json:"name,omitzero"`
	FullyQualifiedName string      `json:"fullyQualifiedName,omitzero"`
	OffsetFromParent   int         `json:"offsetFromParent,omitzero"`
	Index              *int        `json:"index,omitzero"`
	ParentIndex        *int        `json:"parentIndex,omitzero"`
	Properties         PropertyBag `json:"properties,omitzero"`
}

type PhysicalLocation struct {
	Address          *Address          `json:"address,omitzero"`
	ArtifactLocation *ArtifactLocation `json:"artifactLocation,omitzero"`
	Region           *Region           `json:"region,omitzero"`
	ContextRegion    *Region           `json:"contextRegion,omitzero"`
	Properties       PropertyBag       `json:"properties,omitzero"`
}

type LogicalLocation struct {
	Name               string      `json:"name,omitzero"`
	Index              *int        `json:"index,omitzero"`
	FullyQualifiedName string      `json:"fullyQualifiedName,omitzero"`
	DecoratedName      string      `json:"decoratedName,omitzero"`
	ParentIndex        *int        `json:"parentIndex,omitzero"`
	Kind               string      `json:"kind,omitzero"`
	Properties         PropertyBag `json:"properties,omitzero"`
}

type Location struct {
	Id               *int                    `json:"id,omitzero"`
	PhysicalLocation *PhysicalLocation       `json:"physicalLocation,omitzero"`
	LogicalLocations []*LogicalLocation      `json:"logicalLocations,omitzero"`
	Message          *Message                `json:"message,omitzero"`
	Annotations      []*Region               `json:"annotations,omitzero"`
	Relationships    []*LocationRelationship `json:"relationships,omitzero"`
	Properties       PropertyBag             `json:"properties,omitzero"`
}

type LocationRelationship struct {
	Target      int         `json:"target"`
	Kinds       []string    `json:"kinds,omitzero"`
	Description *Message    `json:"description,omitzero"`
	Properties  PropertyBag `json:"properties,omitzero"`
}

type Artifact struct {
	Description         *Message          `json:"description,omitzero"`
	Location            *ArtifactLocation `json:"location,omitzero"`
	ParentIndex         *int              `json:"parentIndex,omitzero"`
	Offset              int               `json:"offset,omitzero"`
	Length              int               `json:"length,omitzero"`
	Roles               []Role            `json:"roles,omitzero"`
	MimeType            string            `json:"mimeType,omitzero"`
	Contents            *ArtifactContent  `json:"contents,omitzero"`
	Encoding            string            `json:"encoding,omitzero"`
	SourceLanguage      string            `json:"sourceLanguage,omitzero"`
	Hashes              map[string]string `json:"hashes,omitzero"`
	LastModifiedTimeUtc string            `json:"lastModifiedTimeUtc,omitzero"`
	Properties          PropertyBag       `json:"properties,omitzero"`
}

type ReportingConfiguration struct {
	Enabled    *bool       `json:"enabled,omitzero"`
	Level      Level       `json:"level,omitzero"`
	Rank       *float64    `json:"rank,omitzero"`
	Parameters PropertyBag `json:"parameters,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type ReportingDescriptorReference struct {
	Id            string                  `json:"id,omitzero"`
	Index         *int                    `json:"index,omitzero"`
	Guid          string                  `json:"guid,omitzero"`
	ToolComponent *ToolComponentReference `json:"toolComponent,omitzero"`
	Properties    PropertyBag             `json:"properties,omitzero"`
}

type ReportingDescriptorRelationship struct {
	Target      *ReportingDescriptorReference `json:"target"`
	Kinds       []string                      `json:"kinds,omitzero"`
	Description *Message                      `json:"description,omitzero"`
	Properties  PropertyBag                   `json:"properties,omitzero"`
}

type ReportingDescriptor struct {
	Id                   string                               `json:"id"`
	DeprecatedIds        []string                             `json:"deprecatedIds,omitzero"`
	Guid                 string                               `json:"guid,omitzero"`
	DeprecatedGuids      []string                             `json:"deprecatedGuids,omitzero"`
	Name                 string                               `json:"name,omitzero"`
	DeprecatedNames      []string                             `json:"deprecatedNames,omitzero"`
	ShortDescription     *MultiformatMessageString            `json:"shortDescription,omitzero"`
	FullDescription      *MultiformatMessageString            `json:"fullDescription,omitzero"`
	MessageStrings       map[string]*MultiformatMessageString `json:"messageStrings,omitzero"`
	DefaultConfiguration *ReportingConfiguration              `json:"defaultConfiguration,omitzero"`
	HelpUri              string                               `json:"helpUri,omitzero"`
	Help                 *MultiformatMessageString            `json:"help,omitzero"`
	Relationships        []*ReportingDescriptorRelationship   `json:"relationships,omitzero"`
	Properties           PropertyBag                          `json:"properties,omitzero"`
}

type ToolComponentReference struct {
	Name       string      `json:"name,omitzero"`
	Index      *int        `json:"index,omitzero"`
	Guid       string      `json:"guid,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type TranslationMetadata struct {
	Name             string                    `json:"name"`
	FullName         string                    `json:"fullName,omitzero"`
	ShortDescription *MultiformatMessageString `json:"shortDescription,omitzero"`
	FullDescription  *MultiformatMessageString `json:"fullDescription,omitzero"`
	DownloadUri      string                    `json:"downloadUri,omitzero"`
	InformationUri   string                    `json:"informationUri,omitzero"`
	Properties       PropertyBag               `json:"properties,omitzero"`
}

type ToolComponent struct {
	Guid                                        string                               `json:"guid,omitzero"`
	Name                                        string                               `json:"name"`
	Organization                                string                               `json:"organization,omitzero"`
	Product                                     string                               `json:"product,omitzero"`
	ProductSuite                                string                               `json:"productSuite,omitzero"`
	ShortDescription                            *MultiformatMessageString            `json:"shortDescription,omitzero"`
	FullDescription                             *MultiformatMessageString            `json:"fullDescription,omitzero"`
	FullName                                    string                               `json:"fullName,omitzero"`
	Version                                     string                               `json:"version,omitzero"`
	SemanticVersion                             string                               `json:"semanticVersion,omitzero"`
	DottedQuadFileVersion                       string                               `json:"dottedQuadFileVersion,omitzero"`
	ReleaseDateUtc                              string                               `json:"releaseDateUtc,omitzero"`
	DownloadUri                                 string                               `json:"downloadUri,omitzero"`
	InformationUri                              string                               `json:"informationUri,omitzero"`
	GlobalMessageStrings                        map[string]*MultiformatMessageString `json:"globalMessageStrings,omitzero"`
	Notifications                               []*ReportingDescriptor               `json:"notifications,omitzero"`
	Rules                                       []*ReportingDescriptor               `json:"rules,omitzero"`
	Taxa                                        []*ReportingDescriptor               `json:"taxa,omitzero"`
	Locations                                   []*ArtifactLocation                  `json:"locations,omitzero"`
	Language                                    string                               `json:"language,omitzero"`
	Contents                                    []string                             `json:"contents,omitzero"`
	IsComprehensive                             *bool                                `json:"isComprehensive,omitzero"`
	LocalizedDataSemanticVersion                string                               `json:"localizedDataSemanticVersion,omitzero"`
	MinimumRequiredLocalizedDataSemanticVersion string                               `json:"minimumRequiredLocalizedDataSemanticVersion,omitzero"`
	AssociatedComponent                         *ToolComponentReference              `json:"associatedComponent,omitzero"`
	TranslationMetadata                         *TranslationMetadata                 `json:"translationMetadata,omitzero"`
	SupportedTaxonomies                         []*ToolComponentReference            `json:"supportedTaxonomies,omitzero"`
	Properties                                  PropertyBag                          `json:"properties,omitzero"`
}

type Tool struct {
	Driver     *ToolComponent   `json:"driver"`
	Extensions []*ToolComponent `json:"extensions,omitzero"`
	Properties PropertyBag      `json:"properties,omitzero"`
}

type Replacement struct {
	DeletedRegion   *Region          `json:"deletedRegion"`
	InsertedContent *ArtifactContent `json:"insertedContent,omitzero"`
	Properties      PropertyBag      `json:"properties,omitzero"`
}

type ArtifactChange struct {
	ArtifactLocation *ArtifactLocation `json:"artifactLocation"`
	Replacements     []*Replacement    `json:"replacements"`
	Properties       PropertyBag       `json:"properties,omitzero"`
}

type Fix struct {
	Description     *Message          `json:"description,omitzero"`
	ArtifactChanges []*ArtifactChange `json:"artifactChanges"`
	Properties      PropertyBag       `json:"properties,omitzero"`
}

type Attachment struct {
	Description      *Message          `json:"description,omitzero"`
	ArtifactLocation *ArtifactLocation `json:"artifactLocation"`
	Regions          []*Region         `json:"regions,omitzero"`
	Rectangles       []*Rectangle      `json:"rectangles,omitzero"`
	Properties       PropertyBag       `json:"properties,omitzero"`
}

type StackFrame struct {
	Location   *Location   `json:"location,omitzero"`
	Module     string      `json:"module,omitzero"`
	ThreadId   *int        `json:"threadId,omitzero"`
	Parameters []string    `json:"parameters,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type Stack struct {
	Message    *Message      `json:"message,omitzero"`
	Frames     []*StackFrame `json:"frames"`
	Properties PropertyBag   `json:"properties,omitzero"`
}

type WebRequest struct {
	Index      *int              `json:"index,omitzero"`
	Protocol   string            `json:"protocol,omitzero"`
	Version    string            `json:"version,omitzero"`
	Target     string            `json:"target,omitzero"`
	Method     string            `json:"method,omitzero"`
	Headers    map[string]string `json:"headers,omitzero"`
	Parameters map[string]string `json:"parameters,omitzero"`
	Body       *ArtifactContent  `json:"body,omitzero"`
	Properties PropertyBag       `json:"properties,omitzero"`
}

type WebResponse struct {
	Index              *int              `json:"index,omitzero"`
	Protocol           string            `json:"protocol,omitzero"`
	Version            string            `json:"version,omitzero"`
	StatusCode         int               `json:"statusCode,omitzero"`
	ReasonPhrase       string            `json:"reasonPhrase,omitzero"`
	Headers            map[string]string `json:"headers,omitzero"`
	Body               *ArtifactContent  `json:"body,omitzero"`
	NoResponseReceived *bool             `json:"noResponseReceived,omitzero"`
	Properties         PropertyBag       `json:"properties,omitzero"`
}

type ThreadFlowLocation struct {
	Index            *int                                 `json:"index,omitzero"`
	Location         *Location                            `json:"location,omitzero"`
	Stack            *Stack                               `json:"stack,omitzero"`
	Kinds            []string                             `json:"kinds,omitzero"`
	Taxa             []*ReportingDescriptorReference      `json:"taxa,omitzero"`
	Module           string                               `json:"module,omitzero"`
	State            map[string]*MultiformatMessageString `json:"state,omitzero"`
	NestingLevel     int                                  `json:"nestingLevel,omitzero"`
	ExecutionOrder   int                                  `json:"executionOrder,omitzero"`
	ExecutionTimeUtc string                               `json:"executionTimeUtc,omitzero"`
	Importance       Importance                           `json:"importance,omitzero"`
	WebRequest       *WebRequest                          `json:"webRequest,omitzero"`
	WebResponse      *WebResponse                         `json:"webResponse,omitzero"`
	Properties       PropertyBag                          `json:"properties,omitzero"`
}

type ThreadFlow struct {
	Id             string                               `json:"id,omitzero"`
	Message        *Message                             `json:"message,omitzero"`
	InitialState   map[string]*MultiformatMessageString `json:"initialState,omitzero"`
	ImmutableState map[string]*MultiformatMessageString `json:"immutableState,omitzero"`
	Locations      []*ThreadFlowLocation                `json:"locations"`
	Properties     PropertyBag                          `json:"properties,omitzero"`
}

type CodeFlow struct {
	Message     *Message      `json:"message,omitzero"`
	ThreadFlows []*ThreadFlow `json:"threadFlows"`
	Properties  PropertyBag   `json:"properties,omitzero"`
}

type EdgeTraversal struct {
	EdgeId            string                               `json:"edgeId"`
	Message           *Message                             `json:"message,omitzero"`
	FinalState        map[string]*MultiformatMessageString `json:"finalState,omitzero"`
	StepOverEdgeCount int                                  `json:"stepOverEdgeCount,omitzero"`
	Properties        PropertyBag                          `json:"properties,omitzero"`
}

type GraphTraversal struct {
	RunGraphIndex    *int                                 `json:"runGraphIndex,omitzero"`
	ResultGraphIndex *int                                 `json:"resultGraphIndex,omitzero"`
	Description      *Message                             `json:"description,omitzero"`
	InitialState     map[string]*MultiformatMessageString `json:"initialState,omitzero"`
	ImmutableState   map[string]*MultiformatMessageString `json:"immutableState,omitzero"`
	EdgeTraversals   []*EdgeTraversal                     `json:"edgeTraversals,omitzero"`
	Properties       PropertyBag                          `json:"properties,omitzero"`
}

type Node struct {
	Id         string      `json:"id"`
	Label      *Message    `json:"label,omitzero"`
	Location   *Location   `json:"location,omitzero"`
	Children   []*Node     `json:"children,omitzero"`
	Properties PropertyBag `json:"properties,omitzero"`
}

type Edge struct {
	Id           string      `json:"id"`
	Label        *Message    `json:"label,omitzero"`
	SourceNodeId string      `json:"sourceNodeId"`
	TargetNodeId string      `json:"targetNodeId"`
	Properties   PropertyBag `json:"properties,omitzero"`
}

type Graph struct {
	Description *Message    `json:"description,omitzero"`
	Nodes       []*Node     `json:"nodes,omitzero"`
	Edges       []*Edge     `json:"edges,omitzero"`
	Properties  PropertyBag `json:"properties,omitzero"`
}

type Suppression struct {
	Guid          string            `json:"guid,omitzero"`
	Kind          SuppressionKind   `json:"kind"`
	Status        SuppressionStatus `json:"status,omitzero"`
	Justification string            `json:"justification,omitzero"`
	Location      *Location         `json:"location,omitzero"`
	Properties    PropertyBag       `json:"properties,omitzero"`
}

type ResultProvenance struct {
	FirstDetectionTimeUtc string              `json:"firstDetectionTimeUtc,omitzero"`
	LastDetectionTimeUtc  string              `json:"lastDetectionTimeUtc,omitzero"`
	FirstDetectionRunGuid string              `json:"firstDetectionRunGuid,omitzero"`
	LastDetectionRunGuid  string              `json:"lastDetectionRunGuid,omitzero"`
	InvocationIndex       *int                `json:"invocationIndex,omitzero"`
	ConversionSources     []*PhysicalLocation `json:"conversionSources,omitzero"`
	Properties            PropertyBag         `json:"properties,omitzero"`
}

type Result struct {
	RuleId              string                          `json:"ruleId,omitzero"`
	RuleIndex           *int                            `json:"ruleIndex,omitzero"`
	Rule                *ReportingDescriptorReference   `json:"rule,omitzero"`
	Kind                Kind                            `json:"kind,omitzero"`
	Level               Level                           `json:"level,omitzero"`
	Message             *Message                        `json:"message"`
	AnalysisTarget      *ArtifactLocation               `json:"analysisTarget,omitzero"`
	Locations           []*Location                     `json:"locations,omitzero"`
	Guid                string                          `json:"guid,omitzero"`
	CorrelationGuid     string                          `json:"correlationGuid,omitzero"`
	OccurrenceCount     int                             `json:"occurrenceCount,omitzero"`
	PartialFingerprints map[string]string               `json:"partialFingerprints,omitzero"`
	Fingerprints        map[string]string               `json:"fingerprints,omitzero"`
	Stacks              []*Stack                        `json:"stacks,omitzero"`
	CodeFlows           []*CodeFlow                     `json:"codeFlows,omitzero"`
	Graphs              []*Graph                        `json:"graphs,omitzero"`
	GraphTraversals     []*GraphTraversal               `json:"graphTraversals,omitzero"`
	RelatedLocations    []*Location                     `json:"relatedLocations,omitzero"`
	Suppressions        []*Suppression                  `json:"suppressions,omitzero"`
	BaselineState       BaselineState                   `json:"baselineState,omitzero"`
	Rank                *float64                        `json:"rank,omitzero"`
	Attachments         []*Attachment                   `json:"attachments,omitzero"`
	HostedViewerUri     string                          `json:"hostedViewerUri,omitzero"`
	WorkItemUris        []string                        `json:"workItemUris,omitzero"`
	Provenance          *ResultProvenance               `json:"provenance,omitzero"`
	Fixes               []*Fix                          `json:"fixes,omitzero"`
	Taxa                []*ReportingDescriptorReference `json:"taxa,omitzero"`
	WebRequest          *WebRequest                     `json:"webRequest,omitzero"`
	WebResponse         *WebResponse                    `json:"webResponse,omitzero"`
	Properties          PropertyBag                     `json:"properties,omitzero"`
}

type Notification struct {
	Locations      []*Location                   `json:"locations,omitzero"`
	Message        *Message                      `json:"message"`
	Level          Level                         `json:"level,omitzero"`
	ThreadId       *int                          `json:"threadId,omitzero"`
	TimeUtc        string                        `json:"timeUtc,omitzero"`
	Exception      *Exception                    `json:"exception,omitzero"`
	Descriptor     *ReportingDescriptorReference `json:"descriptor,omitzero"`
	AssociatedRule *ReportingDescriptorReference `json:"associatedRule,omitzero"`
	Properties     PropertyBag                   `json:"properties,omitzero"`
}

type Exception struct {
	Kind            string       `json:"kind,omitzero"`
	Message         string       `json:"message,omitzero"`
	Stack           *Stack       `json:"stack,omitzero"`
	InnerExceptions []*Exception `json:"innerExceptions,omitzero"`
	Properties      PropertyBag  `json:"properties,omitzero"`
}

type ConfigurationOverride struct {
	Configuration *ReportingConfiguration       `json:"configuration"`
	Descriptor    *ReportingDescriptorReference `json:"descriptor"`
	Properties    PropertyBag                   `json:"properties,omitzero"`
}

type Invocation struct {
	CommandLine                        string                   `json:"commandLine,omitzero"`
	Arguments                          []string                 `json:"arguments,omitzero"`
	ResponseFiles                      []*ArtifactLocation      `json:"responseFiles,omitzero"`
	StartTimeUtc                       string                   `json:"startTimeUtc,omitzero"`
	EndTimeUtc                         string                   `json:"endTimeUtc,omitzero"`
	ExitCode                           *int                     `json:"exitCode,omitzero"`
	RuleConfigurationOverrides         []*ConfigurationOverride `json:"ruleConfigurationOverrides,omitzero"`
	NotificationConfigurationOverrides []*ConfigurationOverride `json:"notificationConfigurationOverrides,omitzero"`
	ToolExecutionNotifications         []*Notification          `json:"toolExecutionNotifications,omitzero"`
	ToolConfigurationNotifications     []*Notification          `json:"toolConfigurationNotifications,omitzero"`
	ExitCodeDescription                string                   `json:"exitCodeDescription,omitzero"`
	ExitSignalName                     string                   `json:"exitSignalName,omitzero"`
	ExitSignalNumber                   *int                     `json:"exitSignalNumber,omitzero"`
	ProcessStartFailureMessage         string                   `json:"processStartFailureMessage,omitzero"`
	ExecutionSuccessful                bool                     `json:"executionSuccessful"`
	MachineName                        string                   `json:"machineName,omitzero"`
	ProcessId                          *int                     `json:"processId,omitzero"`
	ExecutableLocation                 *ArtifactLocation        `json:"executableLocation,omitzero"`
	WorkingDirectory                   *ArtifactLocation        `json:"workingDirectory,omitzero"`
	EnvironmentVariables               map[string]string        `json:"environmentVariables,omitzero"`
	Stdin                              *ArtifactLocation        `json:"stdin,omitzero"`
	Stdout                             *ArtifactLocation        `json:"stdout,omitzero"`
	Stderr                             *ArtifactLocation        `json:"stderr,omitzero"`
	StdoutStderr                       *ArtifactLocation        `json:"stdoutStderr,omitzero"`
	Account                            string                   `json:"account,omitzero"`
	Locale                             string                   `json:"locale,omitzero"`
	Properties                         PropertyBag              `json:"properties,omitzero"`
}

type Conversion struct {
	Tool                 *Tool               `json:"tool"`
	Invocation           *Invocation         `json:"invocation,omitzero"`
	AnalysisToolLogFiles []*ArtifactLocation `json:"analysisToolLogFiles,omitzero"`
	Properties           PropertyBag         `json:"properties,omitzero"`
}

type VersionControlDetails struct {
	RepositoryUri string            `json:"repositoryUri"`
	RevisionId    string            `json:"revisionId,omitzero"`
	Branch        string            `json:"branch,omitzero"`
	RevisionTag   string            `json:"revisionTag,omitzero"`
	AsOfTimeUtc   string            `json:"asOfTimeUtc,omitzero"`
	MappedTo      *ArtifactLocation `json:"mappedTo,omitzero"`
	Properties    PropertyBag       `json:"properties,omitzero"`
}

type RunAutomationDetails struct {
	Description     *Message    `json:"description,omitzero"`
	Id              string      `json:"id,omitzero"`
	Guid            string      `json:"guid,omitzero"`
	CorrelationGuid string      `json:"correlationGuid,omitzero"`
	Properties      PropertyBag `json:"properties,omitzero"`
}

type ExternalPropertyFileReference struct {
	Location   *ArtifactLocation `json:"location,omitzero"`
	Guid       string            `json:"guid,omitzero"`
	ItemCount  int               `json:"itemCount,omitzero"`
	Properties PropertyBag       `json:"properties,omitzero"`
}

type ExternalPropertyFileReferences struct {
	Conversion             *ExternalPropertyFileReference   `json:"conversion,omitzero"`
	Graphs                 []*ExternalPropertyFileReference `json:"graphs,omitzero"`
	ExternalizedProperties *ExternalPropertyFileReference   `json:"externalizedProperties,omitzero"`
	Artifacts              []*ExternalPropertyFileReference `json:"artifacts,omitzero"`
	Invocations            []*ExternalPropertyFileReference `json:"invocations,omitzero"`
	LogicalLocations       []*ExternalPropertyFileReference `json:"logicalLocations,omitzero"`
	ThreadFlowLocations    []*ExternalPropertyFileReference `json:"threadFlowLocations,omitzero"`
	Results                []*ExternalPropertyFileReference `json:"results,omitzero"`
	Taxonomies             []*ExternalPropertyFileReference `json:"taxonomies,omitzero"`
	Addresses              []*ExternalPropertyFileReference `json:"addresses,omitzero"`
	Driver                 *ExternalPropertyFileReference   `json:"driver,omitzero"`
	Extensions             []*ExternalPropertyFileReference `json:"extensions,omitzero"`
	Policies               []*ExternalPropertyFileReference `json:"policies,omitzero"`
	Translations           []*ExternalPropertyFileReference `json:"translations,omitzero"`
	WebRequests            []*ExternalPropertyFileReference `json:"webRequests,omitzero"`
	WebResponses           []*ExternalPropertyFileReference `json:"webResponses,omitzero"`
	Properties             PropertyBag                      `json:"properties,omitzero"`
}

type SpecialLocations struct {
	DisplayBase *ArtifactLocation `json:"displayBase,omitzero"`
	Properties  PropertyBag       `json:"properties,omitzero"`
}

type Run struct {
	Tool                           *Tool                           `json:"tool"`
	Invocations                    []*Invocation                   `json:"invocations,omitzero"`
	Conversion                     *Conversion                     `json:"conversion,omitzero"`
	Language                       string                          `json:"language,omitzero"`
	VersionControlProvenance       []*VersionControlDetails        `json:"versionControlProvenance,omitzero"`
	OriginalUriBaseIds             map[string]*ArtifactLocation    `json:"originalUriBaseIds,omitzero"`
	Artifacts                      []*Artifact                     `json:"artifacts,omitzero"`
	LogicalLocations               []*LogicalLocation              `json:"logicalLocations,omitzero"`
	Graphs                         []*Graph                        `json:"graphs,omitzero"`
	Results                        []*Result                       `json:"results,omitzero"`
	AutomationDetails              *RunAutomationDetails           `json:"automationDetails,omitzero"`
	RunAggregates                  []*RunAutomationDetails         `json:"runAggregates,omitzero"`
	BaselineGuid                   string                          `json:"baselineGuid,omitzero"`
	RedactionTokens                []string                        `json:"redactionTokens,omitzero"`
	DefaultEncoding                string                          `json:"defaultEncoding,omitzero"`
	DefaultSourceLanguage          string                          `json:"defaultSourceLanguage,omitzero"`
	NewlineSequences               []string                        `json:"newlineSequences,omitzero"`
	ColumnKind                     ColumnKind                      `json:"columnKind,omitzero"`
	ExternalPropertyFileReferences *ExternalPropertyFileReferences `json:"externalPropertyFileReferences,omitzero"`
	ThreadFlowLocations            []*ThreadFlowLocation           `json:"threadFlowLocations,omitzero"`
	Taxonomies                     []*ToolComponent                `json:"taxonomies,omitzero"`
	Addresses                      []*Address                      `json:"addresses,omitzero"`
	Translations                   []*ToolComponent                `json:"translations,omitzero"`
	Policies                       []*ToolComponent                `json:"policies,omitzero"`
	WebRequests                    []*WebRequest                   `json:"webRequests,omitzero"`
	WebResponses                   []*WebResponse                  `json:"webResponses,omitzero"`
	SpecialLocations               *SpecialLocations               `json:"specialLocations,omitzero"`
	Properties                     PropertyBag                     `json:"properties,omitzero"`
}

type ExternalProperties struct {
	Schema                 string                `json:"schema,omitzero"`
	Version                string                `json:"version,omitzero"`
	Guid                   string                `json:"guid,omitzero"`
	RunGuid                string                `json:"runGuid,omitzero"`
	Conversion             *Conversion           `json:"conversion,omitzero"`
	Graphs                 []*Graph              `json:"graphs,omitzero"`
	ExternalizedProperties PropertyBag           `json:"externalizedProperties,omitzero"`
	Artifacts              []*Artifact           `json:"artifacts,omitzero"`
	Invocations            []*Invocation         `json:"invocations,omitzero"`
	LogicalLocations       []*LogicalLocation    `json:"logicalLocations,omitzero"`
	ThreadFlowLocations    []*ThreadFlowLocation `json:"threadFlowLocations,omitzero"`
	Results                []*Result             `json:"results,omitzero"`
	Taxonomies             []*ToolComponent      `json:"taxonomies,omitzero"`
	Driver                 *ToolComponent        `json:"driver,omitzero"`
	Extensions             []*ToolComponent      `json:"extensions,omitzero"`
	Policies               []*ToolComponent      `json:"policies,omitzero"`
	Translations           []*ToolComponent      `json:"translations,omitzero"`
	Addresses              []*Address            `json:"addresses,omitzero"`
	WebRequests            []*WebRequest         `json:"webRequests,omitzero"`
	WebResponses           []*WebResponse        `json:"webResponses,omitzero"`
	Properties             PropertyBag           `json:"properties,omitzero"`
}

type Log struct {
	Schema                   string                `json:"$schema,omitzero"`
	Version                  string                `json:"version"`
	Runs                     []*Run                `json:"runs"`
	InlineExternalProperties []*ExternalProperties `json:"inlineExternalProperties,omitzero"`
	Properties               PropertyBag           `json:"properties,omitzero"`
}
