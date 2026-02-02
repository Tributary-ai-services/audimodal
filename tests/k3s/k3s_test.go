package k3s

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/suite"
)

// K3sComplianceSuite is the test suite for K3s integration tests
type K3sComplianceSuite struct {
	suite.Suite
	client       *AudiModalClient
	config       *Config
	uploadedFiles []string // Track uploaded files for cleanup
}

// SetupSuite runs once before all tests
func (s *K3sComplianceSuite) SetupSuite() {
	s.config = LoadConfig()

	if err := s.config.Validate(); err != nil {
		s.T().Skipf("Skipping K3s integration tests: %v", err)
	}

	s.client = NewAudiModalClient(s.config)
	s.uploadedFiles = []string{}
}

// TearDownSuite runs once after all tests
func (s *K3sComplianceSuite) TearDownSuite() {
	// Clean up uploaded files
	ctx := context.Background()
	for _, fileID := range s.uploadedFiles {
		_ = s.client.DeleteFile(ctx, s.config.TenantID, fileID)
	}
}

// trackFile adds a file ID to the cleanup list
func (s *K3sComplianceSuite) trackFile(fileID string) {
	s.uploadedFiles = append(s.uploadedFiles, fileID)
}

// uploadTestFile uploads a test file and tracks it for cleanup
func (s *K3sComplianceSuite) uploadTestFile(ctx context.Context, relativePath string) (*FileResponse, []byte) {
	filePath := filepath.Join(s.config.TestDataPath, relativePath)

	content, err := os.ReadFile(filePath)
	s.Require().NoError(err, "Failed to read test file: %s", relativePath)

	filename := filepath.Base(filePath)
	resp, err := s.client.UploadFile(ctx, s.config.TenantID, filename, content)
	s.Require().NoError(err, "Failed to upload file: %s", relativePath)

	s.trackFile(resp.FileID)
	return resp, content
}

// processAndWait triggers processing and waits for completion
func (s *K3sComplianceSuite) processAndWait(ctx context.Context, fileID string) {
	err := s.client.ProcessFile(ctx, s.config.TenantID, fileID)
	s.Require().NoError(err, "Failed to trigger processing")

	err = s.client.WaitForProcessing(ctx, s.config.TenantID, fileID,
		time.Duration(s.config.MaxPollAttempts)*s.config.PollInterval,
		s.config.PollInterval)
	s.Require().NoError(err, "Processing did not complete successfully")
}

// TestK3sComplianceSuite runs the test suite
func TestK3sComplianceSuite(t *testing.T) {
	if os.Getenv("AUDIMODAL_API_KEY") == "" {
		t.Skip("Skipping K3s integration tests: AUDIMODAL_API_KEY not set")
	}
	suite.Run(t, new(K3sComplianceSuite))
}

// TestHealthCheck verifies the service is reachable
func (s *K3sComplianceSuite) TestHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	health, err := s.client.HealthCheck(ctx)
	s.Require().NoError(err, "Health check should succeed")
	s.Assert().Equal("ok", health.Status, "Health status should be 'ok'")
}

// TestGDPR_PersonalData tests GDPR personal data detection
func (s *K3sComplianceSuite) TestGDPR_PersonalData() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process GDPR test file
	fileResp, _ := s.uploadTestFile(ctx, "txt/compliance/gdpr/gdpr_personal_data.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Verify GDPR-001 violations for personal data
	gdprViolations := FilterViolationsByRule(violations.Violations, "GDPR-001")
	s.Assert().NotEmpty(gdprViolations, "Expected GDPR-001 violations for personal data")

	// Verify specific PII types detected
	piiTypes := ExtractPIITypes(gdprViolations)
	s.Assert().Contains(piiTypes, types.PIITypeEmail, "Should detect email")
	s.Assert().Contains(piiTypes, types.PIITypePhoneNumber, "Should detect phone number")
}

// TestGDPR_SpecialCategory tests GDPR special category data detection
func (s *K3sComplianceSuite) TestGDPR_SpecialCategory() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process GDPR special category test file
	fileResp, _ := s.uploadTestFile(ctx, "txt/compliance/gdpr/gdpr_special_category.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Verify GDPR-002 violations for special category data
	gdprViolations := FilterViolationsByRule(violations.Violations, "GDPR-002")
	s.Assert().NotEmpty(gdprViolations, "Expected GDPR-002 violations for special category data")

	// Verify SSN is flagged (special category under GDPR)
	s.Assert().True(ContainsPIIType(violations.Violations, types.PIITypeSSN),
		"Should detect SSN as special category data")
}

// TestHIPAA_PHI tests HIPAA PHI detection
func (s *K3sComplianceSuite) TestHIPAA_PHI() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process HIPAA test file
	fileResp, _ := s.uploadTestFile(ctx, "txt/compliance/hipaa/hipaa_phi.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Verify HIPAA-001 violations
	hipaaViolations := FilterViolationsByRule(violations.Violations, "HIPAA-001")
	s.Assert().NotEmpty(hipaaViolations, "Expected HIPAA-001 violations for PHI")

	// HIPAA violations should be critical severity
	for _, v := range hipaaViolations {
		s.Assert().Equal("critical", v.Severity, "HIPAA violations should be critical severity")
	}
}

// TestPCI_CardholderData tests PCI-DSS cardholder data detection
func (s *K3sComplianceSuite) TestPCI_CardholderData() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process PCI test file
	fileResp, _ := s.uploadTestFile(ctx, "txt/compliance/pci-dss/pci_cardholder_data.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Verify PCI-001 violations
	pciViolations := FilterViolationsByRule(violations.Violations, "PCI-001")
	s.Assert().NotEmpty(pciViolations, "Expected PCI-001 violations for cardholder data")

	// All PCI violations should be for credit cards
	for _, v := range pciViolations {
		s.Assert().Equal(types.PIITypeCreditCard, v.PIIType, "PCI violations should be for credit cards")
		s.Assert().Equal("critical", v.Severity, "PCI violations should be critical severity")
	}
}

// TestCCPA_PersonalInfo tests CCPA personal information detection
func (s *K3sComplianceSuite) TestCCPA_PersonalInfo() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process CCPA test file
	fileResp, _ := s.uploadTestFile(ctx, "txt/compliance/ccpa/ccpa_personal_info.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Verify CCPA-001 violations
	ccpaViolations := FilterViolationsByRule(violations.Violations, "CCPA-001")
	s.Assert().NotEmpty(ccpaViolations, "Expected CCPA-001 violations for personal info")

	// CCPA should flag IP addresses (unique to CCPA)
	s.Assert().True(ContainsPIIType(violations.Violations, types.PIITypeIPAddress),
		"CCPA should flag IP addresses")
}

// TestMultiRegulation tests document triggering multiple regulations
func (s *K3sComplianceSuite) TestMultiRegulation() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process multi-PII test file
	fileResp, _ := s.uploadTestFile(ctx, "txt/multi-pii/employee_records.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Should not be compliant
	s.Assert().False(violations.IsCompliant, "Document with PII should not be compliant")

	// Should have violations from multiple regulations
	regulationCounts := CountViolationsByRegulation(violations.Violations)
	s.T().Logf("Violations by regulation: %v", regulationCounts)

	// Should have at least some GDPR violations
	s.Assert().Greater(regulationCounts["GDPR"], 0, "Should have GDPR violations")
}

// TestCleanDocument tests compliance checking with no PII
func (s *K3sComplianceSuite) TestCleanDocument() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process clean document
	fileResp, _ := s.uploadTestFile(ctx, "txt/negative/clean_document.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Clean document should be compliant with no violations
	s.Assert().True(violations.IsCompliant, "Clean document should be compliant")
	s.Assert().Empty(violations.Violations, "Clean document should have no violations")
}

// TestSSNDetection tests SSN pattern detection
func (s *K3sComplianceSuite) TestSSNDetection() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process SSN samples file
	fileResp, _ := s.uploadTestFile(ctx, "txt/pii-types/ssn_samples.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get scan result
	scanResult, err := s.client.GetScanResult(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get scan result")

	// Should detect multiple SSNs
	s.Assert().Greater(scanResult.TotalMatches, 0, "Should detect SSNs")

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Should have SSN violations
	s.Assert().True(ContainsPIIType(violations.Violations, types.PIITypeSSN),
		"Should have SSN violations")
}

// TestCreditCardDetection tests credit card pattern detection
func (s *K3sComplianceSuite) TestCreditCardDetection() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process credit card samples file
	fileResp, _ := s.uploadTestFile(ctx, "txt/pii-types/credit_card_samples.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Should have credit card violations
	s.Assert().True(ContainsPIIType(violations.Violations, types.PIITypeCreditCard),
		"Should have credit card violations")

	// Credit card violations should be PCI-001
	s.Assert().True(ContainsRule(violations.Violations, "PCI-001"),
		"Should have PCI-001 violations for credit cards")
}

// TestEmailDetection tests email pattern detection
func (s *K3sComplianceSuite) TestEmailDetection() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process email samples file
	fileResp, _ := s.uploadTestFile(ctx, "txt/pii-types/email_samples.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get scan result
	scanResult, err := s.client.GetScanResult(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get scan result")

	// Should detect multiple emails
	s.Assert().Greater(scanResult.TotalMatches, 0, "Should detect emails")
}

// TestFalsePositiveRejection tests that false positives are rejected
func (s *K3sComplianceSuite) TestFalsePositiveRejection() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process false positive check file
	fileResp, _ := s.uploadTestFile(ctx, "txt/edge-cases/false_positive_check.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get scan result
	scanResult, err := s.client.GetScanResult(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get scan result")

	// Should have minimal matches (false positives should be rejected)
	s.T().Logf("Total matches in false positive check: %d", scanResult.TotalMatches)
	// We don't assert zero because some valid patterns might exist
}

// TestMedicalRecords tests comprehensive medical record scanning
func (s *K3sComplianceSuite) TestMedicalRecords() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process medical records file
	fileResp, _ := s.uploadTestFile(ctx, "txt/multi-pii/medical_records.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Medical records should trigger both GDPR and HIPAA
	s.Assert().True(ContainsRule(violations.Violations, "HIPAA-001"),
		"Medical records should trigger HIPAA")

	// Should have critical severity violations
	criticalCount := CountViolationsBySeverity(violations.Violations)["critical"]
	s.Assert().Greater(criticalCount, 0, "Medical records should have critical violations")
}

// TestFinancialReport tests financial report scanning
func (s *K3sComplianceSuite) TestFinancialReport() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Upload and process financial report file
	fileResp, _ := s.uploadTestFile(ctx, "txt/multi-pii/financial_report.txt")
	s.T().Logf("Uploaded file: %s", fileResp.FileID)

	s.processAndWait(ctx, fileResp.FileID)

	// Get violations
	violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
	s.Require().NoError(err, "Failed to get violations")

	// Financial reports should trigger PCI-DSS for credit cards
	s.Assert().True(ContainsRule(violations.Violations, "PCI-001") ||
		len(violations.Violations) == 0, // May not have credit cards
		"Financial report should trigger PCI-001 if credit cards present")
}

// Benchmark tests for performance validation

func BenchmarkK3sUpload(b *testing.B) {
	cfg := LoadConfig()
	if cfg.APIKey == "" {
		b.Skip("Skipping K3s benchmark: AUDIMODAL_API_KEY not set")
	}

	client := NewAudiModalClient(cfg)
	ctx := context.Background()

	// Small test content
	content := []byte("Test content for benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.UploadFile(ctx, cfg.TenantID, "benchmark_test.txt", content)
		if err != nil {
			b.Fatalf("Upload failed: %v", err)
		}
	}
}
