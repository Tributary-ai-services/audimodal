package tiering

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AccessAnalyzer analyzes file access patterns for intelligent tiering recommendations
type AccessAnalyzer struct {
	config       *TieringConfig
	metricsStore MetricsStore
	tracer       trace.Tracer
}

// NewAccessAnalyzer creates a new access analyzer
func NewAccessAnalyzer(config *TieringConfig, metricsStore MetricsStore) *AccessAnalyzer {
	return &AccessAnalyzer{
		config:       config,
		metricsStore: metricsStore,
		tracer:       otel.Tracer("access-analyzer"),
	}
}

// GenerateRecommendations generates tiering recommendations for a tenant
func (aa *AccessAnalyzer) GenerateRecommendations(ctx context.Context, tenantID uuid.UUID) ([]*TieringRecommendation, error) {
	ctx, span := aa.tracer.Start(ctx, "generate_tiering_recommendations")
	defer span.End()

	span.SetAttributes(attribute.String("tenant.id", tenantID.String()))

	var allRecommendations []*TieringRecommendation

	// Analyze files by current access pattern
	patterns := []TierAccessPattern{
		AccessPatternFrequent, AccessPatternRegular, AccessPatternInfrequent,
		AccessPatternRare, AccessPatternArchival,
	}

	for _, pattern := range patterns {
		files, err := aa.metricsStore.ListFilesByAccessPattern(ctx, pattern, 1000)
		if err != nil {
			span.RecordError(err)
			continue
		}

		recommendations := aa.analyzeFilesForPattern(ctx, files, pattern)
		allRecommendations = append(allRecommendations, recommendations...)
	}

	// Filter and rank recommendations
	filteredRecommendations := aa.filterRecommendations(allRecommendations)
	rankedRecommendations := aa.rankRecommendations(filteredRecommendations)

	span.SetAttributes(
		attribute.Int("recommendations.total", len(allRecommendations)),
		attribute.Int("recommendations.filtered", len(rankedRecommendations)),
	)

	return rankedRecommendations, nil
}

// analyzeFilesForPattern analyzes files with a specific access pattern
func (aa *AccessAnalyzer) analyzeFilesForPattern(ctx context.Context, files []*FileAccessMetrics, pattern TierAccessPattern) []*TieringRecommendation {
	var recommendations []*TieringRecommendation

	for _, file := range files {
		recommendation := aa.analyzeFileForTiering(ctx, file)
		if recommendation != nil {
			recommendations = append(recommendations, recommendation)
		}
	}

	return recommendations
}

// analyzeFileForTiering analyzes a single file for tiering opportunities
func (aa *AccessAnalyzer) analyzeFileForTiering(ctx context.Context, file *FileAccessMetrics) *TieringRecommendation {
	ctx, span := aa.tracer.Start(ctx, "analyze_file_for_tiering")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.path", file.FilePath),
		attribute.String("current.tier", string(file.CurrentTier)),
		attribute.String("access.pattern", string(file.AccessFrequency)),
	)

	// Determine optimal tier based on access pattern
	optimalTier := aa.determineOptimalTier(file)
	
	// Skip if already in optimal tier
	if optimalTier == file.CurrentTier {
		return nil
	}

	// Calculate potential savings and transition cost
	costAnalysis := aa.calculateCostAnalysis(file, optimalTier)
	
	// Skip if not cost-effective
	if costAnalysis.MonthlySavings <= 0 || costAnalysis.PaybackMonths > 12 {
		return nil
	}

	// Calculate confidence score
	confidenceScore := aa.calculateConfidenceScore(file, optimalTier)

	// Generate recommendation
	recommendation := &TieringRecommendation{
		ID:               uuid.New(),
		FilePath:         file.FilePath,
		CurrentTier:      file.CurrentTier,
		RecommendedTier:  optimalTier,
		Reason:           aa.generateReasonText(file, optimalTier),
		ConfidenceScore:  confidenceScore,
		PotentialSavings: costAnalysis.MonthlySavings,
		TransitionCost:   costAnalysis.TransitionCost,
		PaybackPeriod:    time.Duration(costAnalysis.PaybackMonths) * 30 * 24 * time.Hour,
		AccessMetrics:    file,
		CostAnalysis:     costAnalysis,
		GeneratedAt:      time.Now(),
		ValidUntil:       time.Now().Add(7 * 24 * time.Hour), // Valid for 7 days
		Status:           "pending",
	}

	span.SetAttributes(
		attribute.String("recommended.tier", string(optimalTier)),
		attribute.Float64("confidence.score", confidenceScore),
		attribute.Float64("potential.savings", costAnalysis.MonthlySavings),
		attribute.Int("payback.months", costAnalysis.PaybackMonths),
	)

	return recommendation
}

// determineOptimalTier determines the optimal tier for a file based on access patterns
func (aa *AccessAnalyzer) determineOptimalTier(file *FileAccessMetrics) StorageTier {
	now := time.Now()
	age := now.Sub(file.FirstAccessed)
	timeSinceLastAccess := now.Sub(file.LastAccessed)

	// Calculate access frequency metrics
	totalDays := math.Max(1, age.Hours()/24)
	accessesPerDay := float64(file.TotalAccesses) / totalDays

	// Factor in file size for cost considerations
	fileSizeGB := float64(file.FileSize) / (1024 * 1024 * 1024)

	// Decision matrix based on access patterns and file characteristics
	switch {
	case accessesPerDay >= 1.0:
		// Very frequent access - keep in hot tier
		return TierHot
		
	case accessesPerDay >= 0.2 && timeSinceLastAccess < 7*24*time.Hour:
		// Regular access and recent - warm tier
		return TierWarm
		
	case accessesPerDay >= 0.03 && timeSinceLastAccess < 30*24*time.Hour:
		// Infrequent but recent access - warm tier
		return TierWarm
		
	case timeSinceLastAccess < 90*24*time.Hour && fileSizeGB < 10:
		// Small files accessed within 90 days - keep in warm
		return TierWarm
		
	case timeSinceLastAccess < 365*24*time.Hour:
		// Accessed within a year - cold tier
		return TierCold
		
	case age > 7*365*24*time.Hour:
		// Very old files - glacier for long-term retention
		return TierGlacier
		
	default:
		// Default to archive for rarely accessed files
		return TierArchive
	}
}

// calculateCostAnalysis calculates the cost impact of tier transition
func (aa *AccessAnalyzer) calculateCostAnalysis(file *FileAccessMetrics, targetTier StorageTier) *CostAnalysis {
	fileSizeGB := float64(file.FileSize) / (1024 * 1024 * 1024)
	
	// Get cost per GB for current and target tiers
	currentCost := aa.getTierCostPerGB(file.CurrentTier)
	targetCost := aa.getTierCostPerGB(targetTier)
	
	// Calculate monthly storage costs
	currentMonthlyCost := fileSizeGB * currentCost
	targetMonthlyCost := fileSizeGB * targetCost
	monthlySavings := currentMonthlyCost - targetMonthlyCost
	
	// Estimate transition cost (varies by provider and transition type)
	transitionCost := aa.getTransitionCost(file.CurrentTier, targetTier, fileSizeGB)
	
	// Calculate payback period
	var paybackMonths int
	if monthlySavings > 0 {
		paybackMonths = int(math.Ceil(transitionCost / monthlySavings))
	} else {
		paybackMonths = 999 // Never pays back
	}
	
	// Estimate retrieval costs based on access pattern
	retrievalCost := aa.estimateRetrievalCost(file, targetTier)
	
	return &CostAnalysis{
		CurrentMonthlyCost:     currentMonthlyCost,
		RecommendedMonthlyCost: targetMonthlyCost + retrievalCost,
		TransitionCost:         transitionCost,
		MonthlySavings:         monthlySavings - retrievalCost,
		AnnualSavings:          (monthlySavings - retrievalCost) * 12,
		ROI:                    ((monthlySavings - retrievalCost) * 12) / transitionCost * 100,
		PaybackMonths:          paybackMonths,
		StorageCost:            targetMonthlyCost,
		RetrievalCost:          retrievalCost,
	}
}

// calculateConfidenceScore calculates confidence in the recommendation
func (aa *AccessAnalyzer) calculateConfidenceScore(file *FileAccessMetrics, targetTier StorageTier) float64 {
	score := 0.0
	
	// Factor 1: Data history length (more history = higher confidence)
	dataAge := time.Since(file.FirstAccessed).Hours() / 24 / 30 // months
	historyScore := math.Min(dataAge/6, 1.0) * 0.3 // Max 6 months for full score
	
	// Factor 2: Access pattern consistency
	consistencyScore := aa.calculateAccessConsistency(file) * 0.3
	
	// Factor 3: File size consideration (larger files = higher confidence in savings)
	fileSizeGB := float64(file.FileSize) / (1024 * 1024 * 1024)
	sizeScore := math.Min(fileSizeGB/1.0, 1.0) * 0.2 // 1GB for full score
	
	// Factor 4: Time since last access
	daysSinceAccess := time.Since(file.LastAccessed).Hours() / 24
	accessScore := 0.0
	switch targetTier {
	case TierWarm:
		accessScore = math.Max(0, 1.0-(daysSinceAccess/30)) * 0.2
	case TierCold:
		accessScore = math.Min(daysSinceAccess/90, 1.0) * 0.2
	case TierArchive:
		accessScore = math.Min(daysSinceAccess/365, 1.0) * 0.2
	case TierGlacier:
		accessScore = math.Min(daysSinceAccess/(2*365), 1.0) * 0.2
	}
	
	score = historyScore + consistencyScore + sizeScore + accessScore
	return math.Min(score, 1.0)
}

// calculateAccessConsistency calculates how consistent the access pattern is
func (aa *AccessAnalyzer) calculateAccessConsistency(file *FileAccessMetrics) float64 {
	// Calculate coefficient of variation for access patterns
	if len(file.DailyAccesses) < 7 {
		return 0.5 // Default moderate consistency for insufficient data
	}
	
	// Calculate mean and standard deviation of daily accesses
	mean := 0.0
	for _, accesses := range file.DailyAccesses {
		mean += float64(accesses)
	}
	mean /= float64(len(file.DailyAccesses))
	
	variance := 0.0
	for _, accesses := range file.DailyAccesses {
		diff := float64(accesses) - mean
		variance += diff * diff
	}
	variance /= float64(len(file.DailyAccesses))
	stdDev := math.Sqrt(variance)
	
	// Calculate coefficient of variation (lower = more consistent)
	var cv float64
	if mean > 0 {
		cv = stdDev / mean
	} else {
		cv = 0
	}
	
	// Convert to consistency score (1 - normalized CV)
	consistency := math.Max(0, 1.0-cv)
	return consistency
}

// getTierCostPerGB returns the cost per GB per month for a tier
func (aa *AccessAnalyzer) getTierCostPerGB(tier StorageTier) float64 {
	costs, exists := aa.config.CostThresholds[tier]
	if !exists {
		// Default costs if not configured
		switch tier {
		case TierHot:
			return 0.023
		case TierWarm:
			return 0.0125
		case TierCold:
			return 0.004
		case TierArchive:
			return 0.00099
		case TierGlacier:
			return 0.0004
		default:
			return 0.023
		}
	}
	return costs
}

// getTransitionCost estimates the cost of transitioning between tiers
func (aa *AccessAnalyzer) getTransitionCost(fromTier, toTier StorageTier, sizeGB float64) float64 {
	// Transition costs vary by provider and transition type
	// These are example costs - would be configured per provider
	transitionCosts := map[string]float64{
		string(TierHot) + "->" + string(TierWarm):    0.0,     // Usually free
		string(TierHot) + "->" + string(TierCold):    0.01,    // $0.01 per GB
		string(TierWarm) + "->" + string(TierCold):   0.0,     // Usually free
		string(TierWarm) + "->" + string(TierArchive): 0.05,   // $0.05 per GB
		string(TierCold) + "->" + string(TierArchive): 0.02,   // $0.02 per GB
		string(TierCold) + "->" + string(TierGlacier): 0.05,   // $0.05 per GB
		string(TierArchive) + "->" + string(TierGlacier): 0.03, // $0.03 per GB
	}
	
	key := string(fromTier) + "->" + string(toTier)
	costPerGB, exists := transitionCosts[key]
	if !exists {
		costPerGB = 0.01 // Default transition cost
	}
	
	return sizeGB * costPerGB
}

// estimateRetrievalCost estimates monthly retrieval costs based on access pattern
func (aa *AccessAnalyzer) estimateRetrievalCost(file *FileAccessMetrics, tier StorageTier) float64 {
	fileSizeGB := float64(file.FileSize) / (1024 * 1024 * 1024)
	
	// Estimate monthly accesses based on historical pattern
	totalDays := math.Max(1, time.Since(file.FirstAccessed).Hours()/24)
	accessesPerMonth := float64(file.TotalAccesses) / totalDays * 30
	
	// Retrieval costs per GB by tier
	retrievalCosts := map[StorageTier]float64{
		TierHot:     0.0,    // No retrieval cost
		TierWarm:    0.01,   // $0.01 per GB
		TierCold:    0.02,   // $0.02 per GB
		TierArchive: 0.03,   // $0.03 per GB
		TierGlacier: 0.05,   // $0.05 per GB
	}
	
	costPerGB, exists := retrievalCosts[tier]
	if !exists {
		costPerGB = 0.0
	}
	
	return fileSizeGB * costPerGB * accessesPerMonth
}

// generateReasonText generates human-readable reason for the recommendation
func (aa *AccessAnalyzer) generateReasonText(file *FileAccessMetrics, targetTier StorageTier) string {
	timeSinceLastAccess := time.Since(file.LastAccessed)
	
	switch targetTier {
	case TierWarm:
		return fmt.Sprintf("File has moderate access pattern with %d accesses in recent period", file.TotalAccesses)
	case TierCold:
		return fmt.Sprintf("File not accessed for %d days, suitable for cold storage", int(timeSinceLastAccess.Hours()/24))
	case TierArchive:
		return fmt.Sprintf("File accessed infrequently (last access %d days ago), good candidate for archival", int(timeSinceLastAccess.Hours()/24))
	case TierGlacier:
		return fmt.Sprintf("File is very old (%d days since last access) and suitable for deep archive", int(timeSinceLastAccess.Hours()/24))
	default:
		return "Recommended based on access pattern analysis"
	}
}

// filterRecommendations filters out recommendations that don't meet criteria
func (aa *AccessAnalyzer) filterRecommendations(recommendations []*TieringRecommendation) []*TieringRecommendation {
	var filtered []*TieringRecommendation
	
	for _, rec := range recommendations {
		// Filter by minimum savings threshold
		if rec.PotentialSavings < 1.0 { // Less than $1 monthly savings
			continue
		}
		
		// Filter by confidence score
		if rec.ConfidenceScore < 0.6 { // Less than 60% confidence
			continue
		}
		
		// Filter by payback period
		if rec.PaybackPeriod > 365*24*time.Hour { // More than 1 year payback
			continue
		}
		
		filtered = append(filtered, rec)
	}
	
	return filtered
}

// rankRecommendations ranks recommendations by potential impact
func (aa *AccessAnalyzer) rankRecommendations(recommendations []*TieringRecommendation) []*TieringRecommendation {
	// Sort by impact score (combination of savings and confidence)
	sort.Slice(recommendations, func(i, j int) bool {
		scoreI := recommendations[i].PotentialSavings * recommendations[i].ConfidenceScore
		scoreJ := recommendations[j].PotentialSavings * recommendations[j].ConfidenceScore
		return scoreI > scoreJ
	})
	
	// Limit to top recommendations
	maxRecommendations := 100
	if len(recommendations) > maxRecommendations {
		recommendations = recommendations[:maxRecommendations]
	}
	
	return recommendations
}

// CostOptimizer optimizes storage costs through intelligent tiering
type CostOptimizer struct {
	config       *TieringConfig
	metricsStore MetricsStore
	tracer       trace.Tracer
}

// NewCostOptimizer creates a new cost optimizer
func NewCostOptimizer(config *TieringConfig, metricsStore MetricsStore) *CostOptimizer {
	return &CostOptimizer{
		config:       config,
		metricsStore: metricsStore,
		tracer:       otel.Tracer("cost-optimizer"),
	}
}

// OptimizeForTenant performs cost optimization analysis for a tenant
func (co *CostOptimizer) OptimizeForTenant(ctx context.Context, tenantID uuid.UUID) (*CostOptimizationResult, error) {
	ctx, span := co.tracer.Start(ctx, "optimize_costs_for_tenant")
	defer span.End()

	span.SetAttributes(attribute.String("tenant.id", tenantID.String()))

	// Get current metrics
	metrics, err := co.metricsStore.GetTieringMetrics(ctx, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get tiering metrics: %w", err)
	}

	// Generate access analyzer for recommendations
	analyzer := NewAccessAnalyzer(co.config, co.metricsStore)
	recommendations, err := analyzer.GenerateRecommendations(ctx, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate recommendations: %w", err)
	}

	// Calculate potential savings
	var totalCurrentCost, totalOptimizedCost float64
	for _, rec := range recommendations {
		if rec.CostAnalysis != nil {
			totalCurrentCost += rec.CostAnalysis.CurrentMonthlyCost
			totalOptimizedCost += rec.CostAnalysis.RecommendedMonthlyCost
		}
	}

	potentialSavings := totalCurrentCost - totalOptimizedCost
	
	// Calculate optimization score
	optimizationScore := co.calculateOptimizationScore(metrics, recommendations)

	result := &CostOptimizationResult{
		TenantID:                tenantID,
		AnalysisDate:            time.Now(),
		CurrentMonthlyCost:      totalCurrentCost,
		OptimizedMonthlyCost:    totalOptimizedCost,
		PotentialMonthlySavings: potentialSavings,
		PotentialAnnualSavings:  potentialSavings * 12,
		Recommendations:         recommendations,
		TierDistribution:        metrics.TierDistribution,
		OptimizationScore:       optimizationScore,
	}

	span.SetAttributes(
		attribute.Float64("current.monthly.cost", totalCurrentCost),
		attribute.Float64("potential.monthly.savings", potentialSavings),
		attribute.Float64("optimization.score", optimizationScore),
		attribute.Int("recommendations.count", len(recommendations)),
	)

	return result, nil
}

// calculateOptimizationScore calculates how well-optimized the current setup is
func (co *CostOptimizer) calculateOptimizationScore(metrics *TieringMetrics, recommendations []*TieringRecommendation) float64 {
	// Base score starts at 100 (perfect optimization)
	score := 100.0
	
	// Deduct points for optimization opportunities
	opportunityPenalty := float64(len(recommendations)) * 2.0 // 2 points per recommendation
	score -= opportunityPenalty
	
	// Deduct points for poor tier distribution
	totalFiles := int64(0)
	for _, stats := range metrics.TierDistribution {
		totalFiles += stats.FileCount
	}
	
	if totalFiles > 0 {
		// Ideal distribution: some in each tier based on typical access patterns
		hotRatio := float64(metrics.TierDistribution[TierHot].FileCount) / float64(totalFiles)
		if hotRatio > 0.3 { // More than 30% in hot is suboptimal
			score -= (hotRatio - 0.3) * 50
		}
	}
	
	// Ensure score is between 0 and 100
	return math.Max(0, math.Min(100, score))
}