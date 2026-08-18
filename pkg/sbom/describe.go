package sbom

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/sbom/image"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

// Sources names what an SBOM is to describe.
type Sources struct {
	// Image is the built image, as the store knows it; it becomes the SBOM's subject and its contents required
	// components.
	Image string
	// Dockerfile is the content of the Dockerfile the image was built from; the images its build stages start from
	// are described as excluded components, the final stage's base is only named.
	Dockerfile []byte
	// GoBinaries are local Go executables whose linked modules to list as required components.
	GoBinaries []string
	// NodeLock is the content of a package-lock.json whose packages to list; development dependencies are excluded
	// components.
	NodeLock []byte
}

// Description is what an SBOM holds before it is assembled: its subject, its components, and what could not be
// listed.
type Description struct {
	Subject    *altshiftSbomTypes.Component
	Components []*altshiftSbomTypes.Component
	// Warnings tell what was seen but not listed, prefixed with the image or input it concerns.
	Warnings []string
}

// Describe reads the sources into a description; images are read from the store.
func Describe(ctx context.Context, store *image.Store, sources *Sources) (*Description, error) {
	if sources == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("sources"))
	}
	if store == nil {
		store = &image.Store{}
	}

	description := &Description{}
	warn := func(reference string, warnings []string) {
		for _, warning := range warnings {
			description.Warnings = append(description.Warnings, reference+": "+warning)
		}
	}

	if sources.Image != "" {
		analysis, err := store.Analyze(ctx, sources.Image)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("analyze image %q: %w", sources.Image, err), sources.Image)
		}
		warn(sources.Image, analysis.Warnings)
		description.Subject = ContainerComponent(sources.Image, analysis, "")
		components, err := ImageComponents(analysis, altshiftSbomTypes.ScopeRequired)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("image components (%s): %w", sources.Image, err), sources.Image)
		}
		description.Components = append(description.Components, components...)
	}

	for _, goBinary := range sources.GoBinaries {
		info, err := buildinfo.ReadFile(goBinary)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("read build info of %q: %w", goBinary, err), goBinary)
		}
		description.Components = append(description.Components, BuildInfoComponents(info, goBinary, altshiftSbomTypes.ScopeRequired)...)
	}

	if len(sources.NodeLock) != 0 {
		components, err := ParseNodePackageLock(sources.NodeLock)
		if err != nil {
			return nil, fmt.Errorf("parse node package lock: %w", err)
		}
		description.Components = append(description.Components, components...)
	}

	if len(sources.Dockerfile) != 0 {
		stages, err := ParseDockerfileStages(sources.Dockerfile)
		if err != nil {
			return nil, fmt.Errorf("parse dockerfile stages: %w", err)
		}
		buildImages, finalBase := DockerfileImages(stages)

		for _, buildImage := range buildImages {
			// A FROM naming a build argument ("${BASE_IMAGE}") cannot be resolved without the build's arguments;
			// it is named as written and its contents left out, which the warning says.
			if strings.Contains(buildImage, "$") {
				warn(buildImage, []string{"image reference holds a build argument; its contents are not listed"})
				description.Components = append(description.Components, ContainerComponent(buildImage, nil, altshiftSbomTypes.ScopeExcluded))
				continue
			}
			analysis, err := store.Analyze(ctx, buildImage)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("analyze build image %q: %w", buildImage, err), buildImage)
			}
			warn(buildImage, analysis.Warnings)
			container := ContainerComponent(buildImage, analysis, altshiftSbomTypes.ScopeExcluded)
			components, err := ImageComponents(analysis, altshiftSbomTypes.ScopeExcluded)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("image components (%s): %w", buildImage, err), buildImage)
			}
			description.Components = append(description.Components, Nest(container, components))
		}
		if finalBase != "" {
			description.Components = append(description.Components, ContainerComponent(finalBase, nil, altshiftSbomTypes.ScopeRequired))
		}
	}

	return description, nil
}

// DescribeJson describes the sources and assembles the CycloneDX JSON of the result.
func DescribeJson(ctx context.Context, store *image.Store, sources *Sources) ([]byte, []string, error) {
	description, err := Describe(ctx, store, sources)
	if err != nil {
		return nil, nil, err
	}
	data, err := GenerateBomJson(description.Subject, description.Components)
	if err != nil {
		return nil, nil, fmt.Errorf("generate bom json: %w", err)
	}
	return data, description.Warnings, nil
}
