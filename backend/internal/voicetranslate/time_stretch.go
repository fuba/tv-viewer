package voicetranslate

import "math"

const (
	timeStretchFrameLength     = 960
	timeStretchSynthesisHop    = 480
	timeStretchSearchTolerance = 240
	timeStretchCandidateStep   = 4
	timeStretchCorrelationStep = 16
)

// timeStretchStereo uses WSOLA to shorten mono speech without raising its pitch.
func timeStretchStereo(input []int16, speed float64) []int16 {
	if len(input) == 0 || len(input)%speechChannels != 0 {
		return nil
	}
	inputFrames := len(input) / speechChannels
	if speed <= 1 || inputFrames < timeStretchFrameLength*3 {
		return append([]int16(nil), input...)
	}
	targetFrames := int(math.Ceil(float64(inputFrames) / speed))
	mono := make([]float64, inputFrames+timeStretchFrameLength)
	for frame := 0; frame < inputFrames; frame++ {
		mono[frame] = float64(input[frame*speechChannels])
	}

	window := make([]float64, timeStretchFrameLength)
	for index := range window {
		window[index] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(index)/float64(timeStretchFrameLength-1))
	}
	accumulated := make([]float64, targetFrames+timeStretchFrameLength)
	normalization := make([]float64, len(accumulated))
	addFrame := func(inputAt, outputAt int) {
		for index, weight := range window {
			if outputAt+index >= len(accumulated) || inputAt+index >= len(mono) {
				break
			}
			accumulated[outputAt+index] += mono[inputAt+index] * weight
			normalization[outputAt+index] += weight
		}
	}

	analysisHop := int(math.Round(float64(timeStretchSynthesisHop) * speed))
	searchTolerance := min(
		timeStretchSearchTolerance,
		max(timeStretchCandidateStep, (analysisHop-timeStretchSynthesisHop)*3/4),
	)
	previousInputAt := 0
	addFrame(previousInputAt, 0)
	for outputAt, expectedInputAt := timeStretchSynthesisHop, analysisHop; outputAt < targetFrames; outputAt, expectedInputAt = outputAt+timeStretchSynthesisHop, expectedInputAt+analysisHop {
		lastInputAt := inputFrames - timeStretchFrameLength
		minimum := max(0, min(lastInputAt, expectedInputAt-searchTolerance))
		maximum := max(0, min(lastInputAt, expectedInputAt+searchTolerance))
		if maximum < minimum {
			maximum = minimum
		}
		referenceAt := previousInputAt + timeStretchSynthesisHop
		inputAt := bestWSOLAAlignment(mono, referenceAt, minimum, maximum)
		addFrame(inputAt, outputAt)
		previousInputAt = inputAt
	}
	output := make([]int16, targetFrames*speechChannels)
	for frame := 0; frame < targetFrames; frame++ {
		value := accumulated[frame]
		if normalization[frame] > 1e-12 {
			value /= normalization[frame]
		} else if frame < inputFrames {
			value = mono[frame]
		}
		sample := clampPCM16(value)
		output[frame*speechChannels] = sample
		output[frame*speechChannels+1] = sample
	}
	// Crossfade the final source window onto the target tail. A regular OLA
	// frame extends past targetFrames, so truncating it alone can lose a final
	// consonant even when the rest of the utterance is mapped correctly.
	tailOutputAt := targetFrames - timeStretchFrameLength
	tailInputAt := inputFrames - timeStretchFrameLength
	for offset := 0; offset < timeStretchFrameLength; offset++ {
		weight := 1.0
		if offset < timeStretchSynthesisHop {
			weight = 0.5 - 0.5*math.Cos(math.Pi*float64(offset)/timeStretchSynthesisHop)
		}
		frame := tailOutputAt + offset
		existing := float64(output[frame*speechChannels])
		value := existing*(1-weight) + mono[tailInputAt+offset]*weight
		sample := clampPCM16(value)
		output[frame*speechChannels] = sample
		output[frame*speechChannels+1] = sample
	}
	return output
}

func bestWSOLAAlignment(input []float64, referenceAt, minimum, maximum int) int {
	if minimum >= maximum || referenceAt < 0 || referenceAt+timeStretchFrameLength > len(input) {
		return minimum
	}
	bestAt := minimum
	bestScore := -2.0
	for candidate := minimum; candidate <= maximum; candidate += timeStretchCandidateStep {
		var correlation, referenceEnergy, candidateEnergy float64
		for offset := 0; offset < timeStretchFrameLength; offset += timeStretchCorrelationStep {
			reference := input[referenceAt+offset]
			value := input[candidate+offset]
			correlation += reference * value
			referenceEnergy += reference * reference
			candidateEnergy += value * value
		}
		score := 0.0
		if referenceEnergy > 0 && candidateEnergy > 0 {
			score = correlation / math.Sqrt(referenceEnergy*candidateEnergy)
		}
		if score > bestScore {
			bestScore = score
			bestAt = candidate
		}
	}
	return bestAt
}

func clampPCM16(value float64) int16 {
	switch {
	case value > math.MaxInt16:
		return math.MaxInt16
	case value < math.MinInt16:
		return math.MinInt16
	default:
		return int16(math.Round(value))
	}
}
