package classification

import (
	"context"
	"math"
	"regexp"
	"strings"
)

// BasicSentimentAnalyzer provides basic sentiment analysis using lexicon-based approach
type BasicSentimentAnalyzer struct {
	name               string
	version            string
	supportedLanguages []Language
	lexicons           map[Language]*SentimentLexicon
}

// SentimentLexicon contains sentiment words and their scores for a language
type SentimentLexicon struct {
	PositiveWords   map[string]float64  // word -> positive score
	NegativeWords   map[string]float64  // word -> negative score
	Intensifiers    map[string]float64  // words that intensify sentiment
	Negators        []string            // words that negate sentiment
	Emoticons       map[string]float64  // emoticons and their scores
}

// NewBasicSentimentAnalyzer creates a new basic sentiment analyzer
func NewBasicSentimentAnalyzer() *BasicSentimentAnalyzer {
	analyzer := &BasicSentimentAnalyzer{
		name:               "basic-sentiment-analyzer",
		version:            "1.0.0",
		supportedLanguages: []Language{LanguageEnglish, LanguageSpanish, LanguageFrench},
		lexicons:           make(map[Language]*SentimentLexicon),
	}
	
	analyzer.initializeLexicons()
	return analyzer
}

// AnalyzeSentiment analyzes the sentiment of the given text
func (s *BasicSentimentAnalyzer) AnalyzeSentiment(ctx context.Context, text string) (*SentimentAnalysis, error) {
	if len(text) == 0 {
		return &SentimentAnalysis{
			Overall: SentimentScore{
				Label:     "neutral",
				Score:     0.0,
				Magnitude: 0.0,
			},
			Confidence: 0.0,
		}, nil
	}
	
	// Use English lexicon as default
	lexicon := s.lexicons[LanguageEnglish]
	
	// Clean and preprocess text
	cleanText := s.preprocessText(text)
	
	// Split into sentences for sentence-level analysis
	sentences := s.splitIntoSentences(cleanText)
	sentenceScores := make([]SentimentScore, len(sentences))
	
	totalScore := 0.0
	totalMagnitude := 0.0
	
	for i, sentence := range sentences {
		score := s.analyzeSentence(sentence, lexicon)
		sentenceScores[i] = score
		totalScore += score.Score
		totalMagnitude += score.Magnitude
	}
	
	// Calculate overall sentiment
	avgScore := 0.0
	avgMagnitude := 0.0
	
	if len(sentences) > 0 {
		avgScore = totalScore / float64(len(sentences))
		avgMagnitude = totalMagnitude / float64(len(sentences))
	}
	
	// Determine overall label
	label := "neutral"
	if avgScore > 0.1 {
		label = "positive"
	} else if avgScore < -0.1 {
		label = "negative"
	}
	
	// Calculate confidence based on magnitude and consistency
	confidence := s.calculateConfidence(sentenceScores, avgMagnitude)
	
	return &SentimentAnalysis{
		Overall: SentimentScore{
			Label:     label,
			Score:     avgScore,
			Magnitude: avgMagnitude,
		},
		Sentences:  sentenceScores,
		Confidence: confidence,
	}, nil
}

// GetSupportedLanguages returns supported languages
func (s *BasicSentimentAnalyzer) GetSupportedLanguages() []Language {
	return s.supportedLanguages
}

// preprocessText cleans and normalizes text for sentiment analysis
func (s *BasicSentimentAnalyzer) preprocessText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)
	
	// Remove extra whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	
	return text
}

// splitIntoSentences splits text into sentences
func (s *BasicSentimentAnalyzer) splitIntoSentences(text string) []string {
	// Simple sentence splitting using punctuation
	sentences := regexp.MustCompile(`[.!?]+\s*`).Split(text, -1)
	
	// Filter out empty sentences
	result := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) > 0 {
			result = append(result, sentence)
		}
	}
	
	// If no sentences found, treat entire text as one sentence
	if len(result) == 0 && len(text) > 0 {
		result = append(result, text)
	}
	
	return result
}

// analyzeSentence analyzes sentiment of a single sentence
func (s *BasicSentimentAnalyzer) analyzeSentence(sentence string, lexicon *SentimentLexicon) SentimentScore {
	words := strings.Fields(sentence)
	if len(words) == 0 {
		return SentimentScore{
			Label:     "neutral",
			Score:     0.0,
			Magnitude: 0.0,
		}
	}
	
	totalScore := 0.0
	totalMagnitude := 0.0
	scoredWords := 0
	
	// Check for emoticons first
	emoticonScore := s.analyzeEmoticons(sentence, lexicon)
	if emoticonScore != 0.0 {
		totalScore += emoticonScore
		totalMagnitude += math.Abs(emoticonScore)
		scoredWords++
	}
	
	// Analyze each word
	for i, word := range words {
		wordScore := 0.0
		
		// Check positive words
		if score, exists := lexicon.PositiveWords[word]; exists {
			wordScore = score
		}
		
		// Check negative words
		if score, exists := lexicon.NegativeWords[word]; exists {
			wordScore = -score // Negative words get negative scores
		}
		
		if wordScore != 0.0 {
			// Apply intensifiers
			intensifier := s.checkIntensifiers(words, i, lexicon)
			wordScore *= intensifier
			
			// Apply negation
			if s.checkNegation(words, i, lexicon) {
				wordScore *= -0.5 // Reduce and flip sentiment
			}
			
			totalScore += wordScore
			totalMagnitude += math.Abs(wordScore)
			scoredWords++
		}
	}
	
	// Normalize scores
	avgScore := 0.0
	avgMagnitude := 0.0
	
	if scoredWords > 0 {
		avgScore = totalScore / float64(scoredWords)
		avgMagnitude = totalMagnitude / float64(scoredWords)
	}
	
	// Clamp score between -1 and 1
	if avgScore > 1.0 {
		avgScore = 1.0
	} else if avgScore < -1.0 {
		avgScore = -1.0
	}
	
	// Clamp magnitude between 0 and 1
	if avgMagnitude > 1.0 {
		avgMagnitude = 1.0
	}
	
	// Determine label
	label := "neutral"
	if avgScore > 0.1 {
		label = "positive"
	} else if avgScore < -0.1 {
		label = "negative"
	}
	
	return SentimentScore{
		Label:     label,
		Score:     avgScore,
		Magnitude: avgMagnitude,
	}
}

// analyzeEmoticons analyzes emoticons in the text
func (s *BasicSentimentAnalyzer) analyzeEmoticons(text string, lexicon *SentimentLexicon) float64 {
	totalScore := 0.0
	count := 0
	
	for emoticon, score := range lexicon.Emoticons {
		if strings.Contains(text, emoticon) {
			totalScore += score
			count++
		}
	}
	
	if count > 0 {
		return totalScore / float64(count)
	}
	
	return 0.0
}

// checkIntensifiers checks for intensifier words near the current word
func (s *BasicSentimentAnalyzer) checkIntensifiers(words []string, index int, lexicon *SentimentLexicon) float64 {
	intensifier := 1.0
	
	// Check previous word
	if index > 0 {
		prevWord := words[index-1]
		if multiplier, exists := lexicon.Intensifiers[prevWord]; exists {
			intensifier *= multiplier
		}
	}
	
	// Check word before previous
	if index > 1 {
		prevPrevWord := words[index-2]
		if multiplier, exists := lexicon.Intensifiers[prevPrevWord]; exists {
			intensifier *= multiplier * 0.8 // Reduced impact for distant intensifiers
		}
	}
	
	return intensifier
}

// checkNegation checks for negation words near the current word
func (s *BasicSentimentAnalyzer) checkNegation(words []string, index int, lexicon *SentimentLexicon) bool {
	// Check previous words (up to 3 words back)
	start := index - 3
	if start < 0 {
		start = 0
	}
	
	for i := start; i < index; i++ {
		word := words[i]
		for _, negator := range lexicon.Negators {
			if word == negator {
				return true
			}
		}
	}
	
	return false
}

// calculateConfidence calculates confidence based on sentence consistency and magnitude
func (s *BasicSentimentAnalyzer) calculateConfidence(sentenceScores []SentimentScore, avgMagnitude float64) float64 {
	if len(sentenceScores) == 0 {
		return 0.0
	}
	
	// Calculate consistency (how similar are sentence sentiments)
	consistency := s.calculateConsistency(sentenceScores)
	
	// Confidence is based on both magnitude and consistency
	confidence := (avgMagnitude * 0.7) + (consistency * 0.3)
	
	// Clamp between 0 and 1
	if confidence > 1.0 {
		confidence = 1.0
	} else if confidence < 0.0 {
		confidence = 0.0
	}
	
	return confidence
}

// calculateConsistency calculates how consistent sentiment is across sentences
func (s *BasicSentimentAnalyzer) calculateConsistency(sentenceScores []SentimentScore) float64 {
	if len(sentenceScores) <= 1 {
		return 1.0 // Single sentence is perfectly consistent
	}
	
	// Calculate variance in sentiment scores
	mean := 0.0
	for _, score := range sentenceScores {
		mean += score.Score
	}
	mean /= float64(len(sentenceScores))
	
	variance := 0.0
	for _, score := range sentenceScores {
		diff := score.Score - mean
		variance += diff * diff
	}
	variance /= float64(len(sentenceScores))
	
	// Convert variance to consistency (lower variance = higher consistency)
	consistency := 1.0 / (1.0 + variance)
	
	return consistency
}

// initializeLexicons initializes sentiment lexicons for different languages
func (s *BasicSentimentAnalyzer) initializeLexicons() {
	// English lexicon
	s.lexicons[LanguageEnglish] = &SentimentLexicon{
		PositiveWords: map[string]float64{
			"amazing":     0.8, "awesome":    0.9, "beautiful":  0.7, "best":       0.8,
			"brilliant":   0.8, "excellent":  0.9, "fantastic":  0.8, "good":       0.6,
			"great":       0.7, "happy":      0.7, "incredible": 0.8, "love":       0.8,
			"perfect":     0.9, "wonderful":  0.8, "outstanding":0.9, "superb":     0.8,
			"marvelous":   0.8, "exceptional":0.9, "magnificent":0.8, "delightful": 0.7,
			"enjoy":       0.6, "like":       0.5, "pleased":    0.6, "satisfied":  0.6,
			"positive":    0.6, "success":    0.7, "win":        0.7, "victory":    0.8,
		},
		NegativeWords: map[string]float64{
			"awful":       0.8, "bad":        0.6, "terrible":   0.9, "horrible":   0.9,
			"disgusting":  0.8, "hate":       0.8, "dislike":    0.6, "sad":        0.6,
			"angry":       0.7, "furious":    0.9, "annoyed":    0.6, "disappointed":0.7,
			"frustrated":  0.7, "upset":      0.7, "worried":    0.6, "concerned":  0.5,
			"problem":     0.5, "issue":      0.5, "error":      0.6, "failure":    0.7,
			"wrong":       0.5, "broken":     0.6, "fail":       0.7, "lose":       0.6,
			"worst":       0.9, "useless":    0.7, "worthless":  0.8, "pathetic":   0.8,
		},
		Intensifiers: map[string]float64{
			"very":        1.5, "extremely":  2.0, "incredibly": 1.8, "absolutely": 1.7,
			"completely":  1.6, "totally":    1.5, "really":     1.3, "quite":      1.2,
			"pretty":      1.2, "rather":     1.1, "somewhat":   0.8, "slightly":   0.7,
			"highly":      1.4, "deeply":     1.4, "truly":      1.3, "genuinely":  1.3,
		},
		Negators: []string{
			"not", "no", "never", "none", "nobody", "nothing", "neither", "nowhere",
			"hardly", "barely", "scarcely", "seldom", "rarely", "without", "lack",
			"don't", "doesn't", "didn't", "won't", "wouldn't", "can't", "cannot",
			"shouldn't", "couldn't", "mustn't", "isn't", "aren't", "wasn't", "weren't",
		},
		Emoticons: map[string]float64{
			":)":    0.5, ":-)":   0.5, ":D":    0.7, ":-D":   0.7, ";)":    0.6,
			";-)":   0.6, ":P":    0.4, ":-P":   0.4, "<3":    0.8, "♥":     0.8,
			":(":   -0.5, ":-(":  -0.5, ":'(":  -0.7, ":,(":  -0.7, ":/":   -0.3,
			":-/":  -0.3, ":@":   -0.7, ":|":   -0.2, ":-|":  -0.2, ">:(":  -0.8,
		},
	}
	
	// Spanish lexicon (basic)
	s.lexicons[LanguageSpanish] = &SentimentLexicon{
		PositiveWords: map[string]float64{
			"bueno":       0.6, "excelente":  0.9, "fantástico": 0.8, "increíble": 0.8,
			"maravilloso": 0.8, "perfecto":   0.9, "genial":     0.7, "estupendo":  0.7,
			"feliz":       0.7, "alegre":     0.7, "contento":   0.6, "satisfecho": 0.6,
			"amor":        0.8, "gustar":     0.5, "encantar":   0.7, "adorar":     0.8,
		},
		NegativeWords: map[string]float64{
			"malo":        0.6, "terrible":   0.9, "horrible":   0.9, "pésimo":     0.8,
			"odiar":       0.8, "disgustar":  0.6, "triste":     0.6, "enojado":    0.7,
			"furioso":     0.9, "molesto":    0.6, "preocupado": 0.6, "problema":   0.5,
			"error":       0.6, "fallar":     0.7, "perder":     0.6, "peor":       0.9,
		},
		Intensifiers: map[string]float64{
			"muy":         1.5, "extremadamente": 2.0, "increíblemente": 1.8, "absolutamente": 1.7,
			"completamente": 1.6, "totalmente": 1.5, "realmente": 1.3, "bastante": 1.2,
		},
		Negators: []string{
			"no", "nunca", "nada", "nadie", "ninguno", "sin", "jamás", "tampoco",
		},
		Emoticons: map[string]float64{
			":)": 0.5, ":D": 0.7, ":(": -0.5, ":/": -0.3,
		},
	}
	
	// French lexicon (basic)
	s.lexicons[LanguageFrench] = &SentimentLexicon{
		PositiveWords: map[string]float64{
			"bon":         0.6, "excellent":  0.9, "fantastique": 0.8, "incroyable": 0.8,
			"merveilleux": 0.8, "parfait":    0.9, "génial":     0.7, "superbe":    0.7,
			"heureux":     0.7, "joyeux":     0.7, "content":    0.6, "satisfait":  0.6,
			"amour":       0.8, "aimer":      0.5, "adorer":     0.8, "plaire":     0.6,
		},
		NegativeWords: map[string]float64{
			"mauvais":     0.6, "terrible":   0.9, "horrible":   0.9, "affreux":    0.8,
			"détester":    0.8, "haïr":       0.8, "triste":     0.6, "fâché":      0.7,
			"furieux":     0.9, "ennuyé":     0.6, "inquiet":    0.6, "problème":   0.5,
			"erreur":      0.6, "échouer":    0.7, "perdre":     0.6, "pire":       0.9,
		},
		Intensifiers: map[string]float64{
			"très":        1.5, "extrêmement": 2.0, "incroyablement": 1.8, "absolument": 1.7,
			"complètement": 1.6, "totalement": 1.5, "vraiment": 1.3, "assez": 1.2,
		},
		Negators: []string{
			"ne", "non", "jamais", "rien", "personne", "aucun", "sans", "guère",
		},
		Emoticons: map[string]float64{
			":)": 0.5, ":D": 0.7, ":(": -0.5, ":/": -0.3,
		},
	}
}