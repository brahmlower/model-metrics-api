package aa

// EvalTokenCount holds token usage counts for a single evaluation run.
type EvalTokenCount struct {
	AnswerTokens    int64 `json:"answerTokens"`
	InputTokens     int64 `json:"inputTokens"`
	OutputTokens    int64 `json:"outputTokens"`
	ReasoningTokens int64 `json:"reasoningTokens"`
}

// GdpvalBreakdown holds GDP-val benchmark breakdown statistics.
type GdpvalBreakdown struct {
	AvgTurns  float64 `json:"avgTurns"`
	Elo       float64 `json:"elo"`
	Lower95CI float64 `json:"lower95ci"`
	Upper95CI float64 `json:"upper95ci"`
}

// OmniscienceDomainScore holds per-domain omniscience evaluation scores.
type OmniscienceDomainScore struct {
	Accuracy          float64 `json:"accuracy"`
	HallucinationRate float64 `json:"hallucinationRate"`
	Omniscience       float64 `json:"omniscience"`
}

// OpennessBreakdown holds model openness and licensing scores.
type OpennessBreakdown struct {
	DataPosttrainAccess   int64   `json:"dataPosttrainAccess"`
	DataPosttrainLicense  int64   `json:"dataPosttrainLicense"`
	DataPretrainAccess    int64   `json:"dataPretrainAccess"`
	DataPretrainLicense   int64   `json:"dataPretrainLicense"`
	MethodologyDisclosure int64   `json:"methodologyDisclosure"`
	MethodologyLicense    int64   `json:"methodologyLicense"`
	OpennessIndex         float64 `json:"opennessIndex"`
	WeightsAccess         int64   `json:"weightsAccess"`
	WeightsLicense        int64   `json:"weightsLicense"`
}

// MultilingualBreakdown holds multilingual benchmark scores by language code.
type MultilingualBreakdown struct {
	Index    float64            `json:"index"`
	Language map[string]float64 `json:"language"`
}
