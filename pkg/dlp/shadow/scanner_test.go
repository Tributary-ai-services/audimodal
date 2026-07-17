package shadow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	gk "github.com/Tributary-ai-services/Gatekeeper/pkg/scan"

	"github.com/jscharber/audimodal/pkg/dlp/types"
)

// gkResult builds a Gatekeeper ScanResult with one finding per patternID, for
// the pure diff unit tests.
func gkResult(patternIDs ...string) *gk.ScanResult {
	r := &gk.ScanResult{}
	for _, id := range patternIDs {
		r.Findings = append(r.Findings, gk.Finding{PatternID: id})
	}
	r.TotalFindings = len(r.Findings)
	return r
}

// fakePrimary is a stand-in audimodal DLPScanner: it returns a fixed result and
// records that it was called. It lets us assert the shadow returns the primary
// result verbatim and never substitutes Gatekeeper's.
type fakePrimary struct {
	result   *types.ScanResult
	err      error
	scanned  int
	gotChunk bool
}

func (f *fakePrimary) ScanContent(_ context.Context, _ string, _ *types.ScanConfig) (*types.ScanResult, error) {
	f.scanned++
	return f.result, f.err
}
func (f *fakePrimary) ScanChunk(_ context.Context, _ *types.ChunkContent, _ *types.ScanConfig) (*types.ScanResult, error) {
	f.gotChunk = true
	return f.result, f.err
}
func (f *fakePrimary) GetSupportedPatterns() []types.PatternInfo {
	return []types.PatternInfo{{Type: types.PIITypeEmail}}
}
func (f *fakePrimary) ValidateConfig(_ *types.ScanConfig) error { return nil }

type capLogger struct{ lines []string }

func (c *capLogger) Printf(format string, args ...any) {
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}
func (c *capLogger) last() string {
	if len(c.lines) == 0 {
		return ""
	}
	return c.lines[len(c.lines)-1]
}

func result(findingTypes ...types.PIIType) *types.ScanResult {
	r := &types.ScanResult{Scanner: "audimodal-fake"}
	for _, t := range findingTypes {
		r.Findings = append(r.Findings, types.Finding{Type: t, Value: "x"})
	}
	r.TotalMatches = len(r.Findings)
	return r
}

// The core invariant: audimodal is authoritative. Whatever Gatekeeper does, the
// returned result is the primary's, byte-for-byte — the shadow cannot change
// what audimodal detects.
func TestShadow_ReturnsPrimaryResultUnchanged(t *testing.T) {
	prim := &fakePrimary{result: result(types.PIITypeEmail)}
	s := New(prim, &capLogger{})

	// Content Gatekeeper would find MORE in than the primary claims.
	out, err := s.ScanContent(context.Background(), "email alice@example.com key AKIAIOSFODNN7EXAMPLE", nil)
	if err != nil {
		t.Fatalf("ScanContent: %v", err)
	}
	if out != prim.result {
		t.Fatal("shadow did not return the primary result object")
	}
	if len(out.Findings) != 1 || out.Findings[0].Type != types.PIITypeEmail {
		t.Fatalf("primary result was altered: %+v", out.Findings)
	}
	if out.Scanner != "audimodal-fake" {
		t.Fatalf("scanner attribution changed: %q", out.Scanner)
	}
}

// A primary error is the real error and short-circuits the shadow (don't diff a
// failed scan).
func TestShadow_PrimaryErrorPropagates_NoShadow(t *testing.T) {
	prim := &fakePrimary{err: errors.New("boom")}
	log := &capLogger{}
	s := New(prim, log)
	_, err := s.ScanContent(context.Background(), "alice@example.com", nil)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("primary error not propagated: %v", err)
	}
	if len(log.lines) != 0 {
		t.Fatalf("shadow ran despite primary error: %v", log.lines)
	}
}

// END-TO-END against the REAL Gatekeeper scanner. This is the point of G6: a
// document that audimodal only partly covers must show the coverage gap.
// audimodal (fake) reports just the email; Gatekeeper detects the email AND the
// AWS access key audimodal has no matcher for. The diff must surface
// gatekeeper_only=[aws_access_key] — the secrets gap that justifies the migration.
func TestShadow_RealGatekeeper_SurfacesSecretsGap(t *testing.T) {
	// audimodal "finds" only the email (its matcher set has no secrets).
	prim := &fakePrimary{result: result(types.PIITypeEmail)}
	log := &capLogger{}
	s := New(prim, log)

	content := "ping alice@example.com — deploy key AKIAIOSFODNN7EXAMPLE do not share"
	if _, err := s.ScanContent(context.Background(), content, nil); err != nil {
		t.Fatalf("ScanContent: %v", err)
	}

	line := log.last()
	if line == "" {
		t.Fatal("no shadow diff logged for content with a secret audimodal can't see")
	}
	if !strings.Contains(line, "gatekeeper_only=") || !strings.Contains(line, "aws_access_key") {
		t.Fatalf("diff did not surface the AWS-key coverage gap: %q", line)
	}
	// The email is found by BOTH engines, so it must be in the agreement bucket,
	// not reported as a gap in either direction.
	if strings.Contains(line, "gatekeeper_only=[") && strings.Contains(strings.SplitN(line, "]", 2)[0], "email") {
		t.Fatalf("email wrongly reported as gatekeeper-only: %q", line)
	}
}

// When both engines agree there is nothing, no line is logged (no noise on the
// overwhelming majority of clean ingested content).
func TestShadow_CleanContent_NoDiffLogged(t *testing.T) {
	prim := &fakePrimary{result: result()} // no findings
	log := &capLogger{}
	s := New(prim, log)
	if _, err := s.ScanContent(context.Background(), "the quarterly report is attached", nil); err != nil {
		t.Fatalf("ScanContent: %v", err)
	}
	if len(log.lines) != 0 {
		t.Fatalf("logged a diff for clean content: %v", log.lines)
	}
}

// Empty content is a no-op shadow (nothing to scan), primary still runs.
func TestShadow_EmptyContentSkipsShadow(t *testing.T) {
	prim := &fakePrimary{result: result()}
	log := &capLogger{}
	s := New(prim, log)
	if _, err := s.ScanContent(context.Background(), "", nil); err != nil {
		t.Fatalf("ScanContent: %v", err)
	}
	if prim.scanned != 1 {
		t.Fatal("primary not called for empty content")
	}
	if len(log.lines) != 0 {
		t.Fatalf("shadow ran on empty content: %v", log.lines)
	}
}

func TestShadow_ChunkPathShadows(t *testing.T) {
	prim := &fakePrimary{result: result(types.PIITypeEmail)}
	log := &capLogger{}
	s := New(prim, log)
	_, err := s.ScanChunk(context.Background(),
		&types.ChunkContent{Content: "key AKIAIOSFODNN7EXAMPLE"}, nil)
	if err != nil {
		t.Fatalf("ScanChunk: %v", err)
	}
	if !prim.gotChunk {
		t.Fatal("primary ScanChunk not called")
	}
	if !strings.Contains(log.last(), "aws_access_key") {
		t.Fatalf("chunk path did not shadow: %q", log.last())
	}
}

// Delegation: the shadow must not change advertised capabilities or validation.
func TestShadow_DelegatesCapabilityMethods(t *testing.T) {
	prim := &fakePrimary{result: result()}
	s := New(prim, &capLogger{})
	if got := s.GetSupportedPatterns(); len(got) != 1 || got[0].Type != types.PIITypeEmail {
		t.Fatalf("GetSupportedPatterns not delegated: %+v", got)
	}
	if err := s.ValidateConfig(nil); err != nil {
		t.Fatalf("ValidateConfig not delegated: %v", err)
	}
}

// Pure unit test of the diff, no scanner: exercises all three buckets.
func TestDiff_Buckets(t *testing.T) {
	prim := result(types.PIITypeEmail, types.PIITypeSSN, types.PIITypeSSN) // email x1, ssn x2
	// Gatekeeper-shaped result: email x1, aws_access_key x1 (no ssn).
	gkRes := gkResult("email", "aws_access_key")

	d := diff(prim, gkRes)
	if _, ok := d.both["email"]; !ok {
		t.Error("email should be in both")
	}
	if d.primaryOnly["ssn"] != 2 {
		t.Errorf("ssn should be audimodal-only x2, got %d", d.primaryOnly["ssn"])
	}
	if d.gatekeeperOnly["aws_access_key"] != 1 {
		t.Errorf("aws_access_key should be gatekeeper-only x1, got %d", d.gatekeeperOnly["aws_access_key"])
	}
	if d.empty() {
		t.Error("diff should not be empty")
	}
	s := d.String()
	for _, want := range []string{"gatekeeper_only=", "aws_access_key", "audimodal_only=", "ssn", "both"} {
		if !strings.Contains(s, want) {
			t.Errorf("diff string missing %q: %s", want, s)
		}
	}
}

func TestDiff_EmptyWhenBothClean(t *testing.T) {
	if !diff(result(), gkResult()).empty() {
		t.Fatal("diff of two empty results should be empty")
	}
}

// canonical() is what makes the diff correct across the two engines' different
// vocabularies. A Gatekeeper PatternID and the equivalent audimodal PIIType must
// normalize to the SAME string, or shared findings get double-counted as gaps.
func TestCanonical(t *testing.T) {
	cases := []struct{ in, want string }{
		// prefix stripping + hyphen→underscore
		{"pii-email", "email"},
		{"email", "email"},
		{"cred-aws-access-key", "aws_access_key"},
		{"pii-credit-card", "credit_card"},
		{"credit_card", "credit_card"},
		{"pii-ip-address", "ip_address"},
		{"pii-drivers-license", "drivers_license"},
		{"injection-sql", "sql"},
		// genuine spelling divergences: Gatekeeper form → audimodal form
		{"pii-phone", "phone_number"},
		{"phone_number", "phone_number"},
		{"pii-dob", "date_of_birth"},
		{"date_of_birth", "date_of_birth"},
		{"pii-mrn", "medical_record_number"},
	}
	for _, c := range cases {
		if got := canonical(c.in); got != c.want {
			t.Errorf("canonical(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// The pairing property: the Gatekeeper ID and the audimodal type for the same
	// concept collapse to one key.
	pairs := [][2]string{
		{"pii-email", "email"},
		{"cred-aws-access-key", "aws_access_key"},
		{"pii-phone", "phone_number"},
		{"pii-credit-card", "credit_card"},
	}
	for _, p := range pairs {
		if canonical(p[0]) != canonical(p[1]) {
			t.Errorf("%q and %q should canonicalize equal, got %q vs %q",
				p[0], p[1], canonical(p[0]), canonical(p[1]))
		}
	}
}
