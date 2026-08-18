package types

type BomFormat string

const (
	BomFormatCycloneDX BomFormat = "CycloneDX"
)

type ComponentType string

const (
	ComponentTypeLibrary         ComponentType = "library"
	ComponentTypeApplication     ComponentType = "application"
	ComponentTypeFramework       ComponentType = "framework"
	ComponentTypeContainer       ComponentType = "container"
	ComponentTypeOperatingSystem ComponentType = "operating-system"
)

// Scope tells whether a component is part of the delivered artifact: required (it ships), optional, or excluded (it
// took part in producing the artifact, e.g. a build image or a development dependency, but does not ship).
type Scope string

const (
	ScopeRequired Scope = "required"
	ScopeOptional Scope = "optional"
	ScopeExcluded Scope = "excluded"
)

// The properties this generator writes on components.
const (
	// PropertyImage names the container image a component was found in (the reference it was analyzed under).
	PropertyImage = "altshift:sbom:image"
	// PropertyImageId carries the image ID (the config digest) of a container component.
	PropertyImageId = "altshift:sbom:image:id"
	// PropertyPath is the absolute path inside the image (or the local path given) a component was read from; a
	// component found in several places carries one property per path.
	PropertyPath = "altshift:sbom:path"
)

type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Component struct {
	Type               ComponentType        `json:"type"`
	Name               string               `json:"name"`
	Version            string               `json:"version,omitzero"`
	Scope              Scope                `json:"scope,omitzero"`
	Purl               string               `json:"purl,omitzero"`
	BomRef             string               `json:"bom-ref,omitzero"`
	Hashes             []*Hash              `json:"hashes,omitzero"`
	Licenses           []*LicenseChoice     `json:"licenses,omitzero"`
	ExternalReferences []*ExternalReference `json:"externalReferences,omitzero"`
	Properties         []*Property          `json:"properties,omitzero"`
	// Components nested in this one, e.g. the packages inside a container image.
	Components []*Component `json:"components,omitzero"`
}

type HashAlgorithm string

const (
	HashAlgorithmSHA256 HashAlgorithm = "SHA-256"
	HashAlgorithmSHA512 HashAlgorithm = "SHA-512"
)

type Hash struct {
	Algorithm HashAlgorithm `json:"alg"`
	Content   string        `json:"content"`
}

type LicenseChoice struct {
	License *License `json:"license,omitzero"`
}

type License struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
}

type ExternalReference struct {
	Type string `json:"type"`
	Url  string `json:"url"`
}

type Tool struct {
	Name    string `json:"name,omitzero"`
	Version string `json:"version,omitzero"`
}

type Metadata struct {
	Tools []*Tool `json:"tools,omitzero"`
	// Component is the subject of the BOM: what the components describe, e.g. the built container image.
	Component *Component `json:"component,omitzero"`
}

type Dependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn,omitzero"`
}

type Bom struct {
	BomFormat    BomFormat     `json:"bomFormat"`
	SpecVersion  string        `json:"specVersion"`
	Version      int           `json:"version"`
	Metadata     *Metadata     `json:"metadata,omitzero"`
	Components   []*Component  `json:"components,omitzero"`
	Dependencies []*Dependency `json:"dependencies,omitzero"`
}
