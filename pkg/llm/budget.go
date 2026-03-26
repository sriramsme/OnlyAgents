package llm

const defaultSafetyMarginPct = 10

// SafetyMargin returns 10% of the context window as a token reserve.
func SafetyMargin(contextWindow int) int {
	return contextWindow * defaultSafetyMarginPct / 100
}

// ComputeOutputBudget returns how many tokens the model can safely emit
// in the next call, given how many input tokens were consumed this call.
// configuredMax=0 means no user-imposed ceiling.
func ComputeOutputBudget(contextWindow, inputTokens, safetyMargin, configuredMax int) int {
	available := contextWindow - inputTokens - safetyMargin
	if available <= 0 {
		available = safetyMargin // last-resort floor
	}
	if configuredMax > 0 && configuredMax < available {
		return configuredMax
	}
	return available
}

// ComputeInputHeadroom returns how many tokens remain for tool results
// and other input after reserving space for the model's output.
func ComputeInputHeadroom(contextWindow, inputTokens, outputBudget, safetyMargin int) int {
	headroom := contextWindow - inputTokens - outputBudget - safetyMargin
	if headroom < 0 {
		return 0
	}
	return headroom
}

// PerResultBudget divides the input headroom evenly across tool calls.
// configuredMax=0 means no user-imposed ceiling.
func PerResultBudget(inputHeadroom, toolCallCount, configuredMax int) int {
	if toolCallCount <= 0 {
		return inputHeadroom
	}
	per := inputHeadroom / toolCallCount
	if configuredMax > 0 && configuredMax < per {
		return configuredMax
	}
	return per
}

// IterationBudget holds the computed token limits for the next loop iteration.
type IterationBudget struct {
	MaxTokens       int // max output tokens for the next LLM call
	PerResultTokens int // max chars per tool result before truncation
}

// ComputeIterationBudget derives both output and per-result budgets from
// the previous call's real usage data. Call this after every LLM response
// that contains tool calls, before the next processToolCalls invocation.
func ComputeIterationBudget(
	contextWindow int,
	safetyMargin int,
	usage Usage,
	toolCallCount int,
	configuredMaxTokens int,
	configuredPerResult int,
) IterationBudget {
	outputBudget := ComputeOutputBudget(contextWindow, usage.InputTokens, safetyMargin, configuredMaxTokens)
	inputHeadroom := ComputeInputHeadroom(contextWindow, usage.InputTokens, outputBudget, safetyMargin)
	perResult := PerResultBudget(inputHeadroom, toolCallCount, configuredPerResult)
	return IterationBudget{
		MaxTokens:       outputBudget,
		PerResultTokens: perResult,
	}
}

// EstimateTokens returns a rough token count for the given text.
// Uses ~4 chars/token for prose and JSON — accurate within ~10–15%
// for English content. Not a substitute for exact tokenization but
// sufficient for budget guards and truncation decisions.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	// Count runes not bytes — multi-byte unicode chars are still ~1 token.
	runes := []rune(s)
	// Base estimate: 1 token per 4 characters.
	estimate := len(runes) / 4
	// JSON-heavy content has more structural characters ({, ", :, ,) that
	// tend to inflate char count without adding proportional tokens.
	// Rough correction: count structural chars and apply a small discount.
	structural := 0
	for _, r := range runes {
		switch r {
		case '{', '}', '[', ']', '"', ':', ',':
			structural++
		}
	}
	if structural > len(runes)/4 {
		// More than 25% structural — likely dense JSON, apply discount.
		estimate = estimate * 9 / 10
	}
	if estimate == 0 {
		return 1
	}
	return estimate
}

// TruncateToTokens truncates s to approximately maxTokens tokens,
// preserving whole words where possible.
func TruncateToTokens(s string, maxTokens int) string {
	if EstimateTokens(s) <= maxTokens {
		return s
	}
	// Target character count from token budget.
	targetChars := maxTokens * 4
	runes := []rune(s)
	if targetChars >= len(runes) {
		return s
	}
	// Walk back to a word boundary to avoid cutting mid-token.
	cut := targetChars
	for cut > 0 && runes[cut] != ' ' && runes[cut] != '\n' {
		cut--
	}
	if cut == 0 {
		cut = targetChars // no boundary found, hard cut
	}
	return string(runes[:cut])
}
