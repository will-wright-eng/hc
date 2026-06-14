package sarif

// SARIF 2.1.0 types — the minimal subset GitHub code scanning consumes. Field
// order here determines JSON key order (json.MarshalIndent is deterministic),
// which the golden test pins. See:
// https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/sarif-support-for-code-scanning

type sarifLog struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []run  `json:"runs"`
}

type run struct {
	Tool              toolWrapper       `json:"tool"`
	AutomationDetails automationDetails `json:"automationDetails"`
	Results           []result          `json:"results"`
}

type toolWrapper struct {
	Driver driver `json:"driver"`
}

type driver struct {
	Name           string                `json:"name"`
	InformationURI string                `json:"informationUri"`
	Version        string                `json:"version,omitempty"`
	Rules          []reportingDescriptor `json:"rules"`
}

type reportingDescriptor struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	ShortDescription     textBlock      `json:"shortDescription"`
	FullDescription      textBlock      `json:"fullDescription"`
	Help                 textBlock      `json:"help"`
	DefaultConfiguration configuration  `json:"defaultConfiguration"`
	Properties           ruleProperties `json:"properties"`
}

type textBlock struct {
	Text string `json:"text"`
}

type configuration struct {
	Level string `json:"level"`
}

type ruleProperties struct {
	Tags []string `json:"tags"`
}

type automationDetails struct {
	ID string `json:"id"`
}

type result struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             textBlock         `json:"message"`
	Locations           []location        `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type location struct {
	PhysicalLocation physicalLocation `json:"physicalLocation"`
}

type physicalLocation struct {
	ArtifactLocation artifactLocation `json:"artifactLocation"`
	Region           region           `json:"region"`
}

type artifactLocation struct {
	URI string `json:"uri"`
}

type region struct {
	StartLine int `json:"startLine"`
}
