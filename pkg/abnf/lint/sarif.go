package lint

import (
	"github.com/altshiftab/utils_go/pkg/sarif"
)

const (
	// driverName is the name the linter reports itself under.
	driverName = "abnf"
	// driverInformationUri locates what the linter is part of.
	driverInformationUri = "https://github.com/altshiftab/utils_go"
)

// Report holds the findings of one grammar definition.
type Report struct {
	// Uri locates the definition that was linted.
	Uri      string
	Findings []*Finding
}

// Sarif renders reports as a SARIF 2.1.0 log, with every check declared as a
// rule of the driver and every finding that can be acted on carrying the
// replacement as a fix.
func Sarif(reports []*Report) *sarif.Log {
	ruleIndex := make(map[RuleId]int, len(rules))
	descriptors := make([]*sarif.ReportingDescriptor, 0, len(rules))
	for i, rule := range rules {
		ruleIndex[rule.Id] = i
		descriptors = append(descriptors, describeRule(rule))
	}

	var results []*sarif.Result
	for _, report := range reports {
		for _, finding := range report.Findings {
			results = append(results, makeResult(report.Uri, finding, ruleIndex))
		}
	}

	return &sarif.Log{
		Schema:  sarif.SchemaUri,
		Version: sarif.Version,
		Runs: []*sarif.Run{
			{
				Tool: &sarif.Tool{
					Driver: &sarif.ToolComponent{
						Name:           driverName,
						InformationUri: driverInformationUri,
						Rules:          descriptors,
					},
				},
				// A grammar definition is US-ASCII throughout, so a column
				// counts bytes, characters and code points alike.
				ColumnKind: sarif.ColumnKindUnicodeCodePoints,
				Results:    results,
			},
		},
	}
}

func describeRule(rule *Rule) *sarif.ReportingDescriptor {
	return &sarif.ReportingDescriptor{
		Id:               string(rule.Id),
		Name:             string(rule.Id),
		ShortDescription: &sarif.MultiformatMessageString{Text: rule.Description},
		DefaultConfiguration: &sarif.ReportingConfiguration{
			Level: rule.Level,
		},
		Properties: sarif.PropertyBag{"category": string(rule.Category)},
	}
}

func makeResult(uri string, finding *Finding, ruleIndex map[RuleId]int) *sarif.Result {
	level := sarif.LevelNote
	if rule := finding.Rule(); rule != nil {
		level = rule.Level
	}

	index := ruleIndex[finding.RuleId]

	result := &sarif.Result{
		RuleId:    string(finding.RuleId),
		RuleIndex: &index,
		Kind:      sarif.KindFail,
		Level:     level,
		Message:   &sarif.Message{Text: finding.Message},
		Locations: []*sarif.Location{
			{PhysicalLocation: &sarif.PhysicalLocation{
				ArtifactLocation: &sarif.ArtifactLocation{Uri: uri},
				Region:           makeRegion(finding),
			}},
		},
	}

	if finding.Fixable() {
		result.Fixes = []*sarif.Fix{
			{
				Description: &sarif.Message{Text: "Rewrite as the linter would."},
				ArtifactChanges: []*sarif.ArtifactChange{
					{
						ArtifactLocation: &sarif.ArtifactLocation{Uri: uri},
						Replacements: []*sarif.Replacement{
							{
								DeletedRegion:   makeRegion(finding),
								InsertedContent: &sarif.ArtifactContent{Text: *finding.Replacement},
							},
						},
					},
				},
			},
		}
	}

	return result
}

// makeRegion locates a finding both by line and column, which a reader
// needs, and by byte offset, which an editor applying a fix needs.
func makeRegion(finding *Finding) *sarif.Region {
	return &sarif.Region{
		StartLine:   finding.Start.Line,
		StartColumn: finding.Start.Column,
		EndLine:     finding.End.Line,
		// A SARIF end column points just past the last byte, as the end
		// offset of a finding does.
		EndColumn:  finding.End.Column,
		ByteOffset: finding.Start.Offset,
		ByteLength: finding.End.Offset - finding.Start.Offset,
	}
}
