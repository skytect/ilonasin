package server

func outputTPS(completionTokens int, latencyMS int64) float64 {
	if completionTokens <= 0 || latencyMS <= 0 {
		return 0
	}
	return float64(completionTokens) / (float64(latencyMS) / 1000)
}

func outputTPSAfterTTFT(completionTokens int, latencyMS, ttftMS int64) float64 {
	if completionTokens <= 0 || latencyMS <= 0 || ttftMS <= 0 || latencyMS <= ttftMS {
		return 0
	}
	return float64(completionTokens) / (float64(latencyMS-ttftMS) / 1000)
}
