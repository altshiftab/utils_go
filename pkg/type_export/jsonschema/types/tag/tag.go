package tag

import (
	"fmt"
	"strconv"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

type Tag struct {
	Name string
	Skip bool
	// AdditionalProperties says whether the object a tag is read for accepts members its
	// properties do not name. It is read from the tag of a struct's blank field, the object
	// rather than any one property being what it describes, and is nil when the tag does not
	// say, leaving the decision to whoever asked.
	AdditionalProperties *bool
	Optional             bool
	MinLength            *int
	MaxLength            *int
	Minimum              *float64
	Maximum              *float64
	MaxItems             *int
	MinItems             *int
	Format               string
	OtherOptions         []string
}

func New(tagString string) (*Tag, error) {
	trimmedTagString := strings.TrimSpace(tagString)
	if trimmedTagString == "" {
		return nil, nil
	}

	var tag Tag

	elements := strings.Split(strings.TrimSpace(trimmedTagString), ",")
	if len(elements) == 0 {
		return nil, nil
	}

	if len(elements) == 1 && elements[0] == "-" {
		tag.Skip = true
		return &tag, nil
	}

	tag.Name = elements[0]

	for _, option := range elements[1:] {
		option = strings.ToLower(strings.TrimSpace(option))
		switch option {
		case "optional":
			tag.Optional = true
		default:
			key, value, ok := strings.Cut(option, ":")
			if ok {
				switch strings.ToLower(key) {
				case "format":
					tag.Format = value
					continue
				case "minlength":
					minLength, err := strconv.Atoi(value)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi (minlength): %w", err))
					}
					tag.MinLength = &minLength
					continue
				case "maxlength":
					maxLength, err := strconv.Atoi(value)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi (maxlength): %w", err))
					}
					tag.MaxLength = &maxLength
					continue
				case "minimum":
					minimum, err := strconv.ParseFloat(value, 64)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(fmt.Errorf("strconv parse float (minimum): %w", err))
					}
					tag.Minimum = &minimum
					continue
				case "maximum":
					maximum, err := strconv.ParseFloat(value, 64)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(fmt.Errorf("strconv parse float (maximum): %w", err))
					}
					tag.Maximum = &maximum
					continue
				case "additionalproperties":
					additionalProperties, err := strconv.ParseBool(value)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(
							fmt.Errorf("strconv parse bool (additionalproperties): %w", err),
						)
					}
					tag.AdditionalProperties = &additionalProperties
					continue
				case "minitems":
					minItems, err := strconv.Atoi(value)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi (minitems): %w", err))
					}
					tag.MinItems = &minItems
					continue
				case "maxitems":
					maxItems, err := strconv.Atoi(value)
					if err != nil {
						return nil, motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi (maxitems): %w", err))
					}
					tag.MaxItems = &maxItems
					continue
				}
			}
			tag.OtherOptions = append(tag.OtherOptions, option)
		}
	}

	return &tag, nil
}
