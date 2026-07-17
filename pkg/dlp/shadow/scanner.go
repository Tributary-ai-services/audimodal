// Package shadow runs Gatekeeper's scanner alongside audimodal's own DLP on the
// same ingested content, logs the per-type finding diff, and returns audimodal's
// result UNCHANGED. It is measure-only: a shadow, not a replacement.
//
// Why (docs: tas-llm-router AIQG-GATEKEEPER-INTEGRATION §4a, audimodal#26):
// Gatekeeper was extracted from audimodal's pkg/dlp and has since drifted well
// past it — 21 PII matchers vs audimodal's 5, plus injection detection and cloud
// SECRETS (AWS/GCP/Azure keys, private keys, connection strings) that audimodal
// cannot detect at all. Before swapping a live 4,700-line DLP for the library
// extracted from it, we dual-run and diff: the shadow surfaces both the coverage
// gain (types only Gatekeeper finds) and any potential regression (types only
// audimodal finds), so the false-positive delta of 5→21 matchers is understood
// before any cutover. Same discipline as the P0 cache probe and the MCP boundary
// scan's log-only stage — measure before switching.
//
// audimodal stays authoritative throughout: the primary scanner's result is
// returned verbatim, and any Gatekeeper error is logged and swallowed. Turning
// this on cannot change what audimodal detects, redacts, or blocks.
package shadow

import (
	"context"
	"sort"
	"strings"

	gk "github.com/Tributary-ai-services/Gatekeeper/pkg/scan"
	"github.com/jscharber/audimodal/pkg/dlp"
	"github.com/jscharber/audimodal/pkg/dlp/types"
)

// Logger is the minimal logging surface the shadow needs; *log.Logger satisfies
// it, so audimodal's stdlib logging drops in without a new dependency.
type Logger interface {
	Printf(format string, args ...any)
}

// Scanner wraps an audimodal DLPScanner and shadows it with Gatekeeper.
// It satisfies dlp.DLPScanner, so it drops into NewDLPServiceWithComponents in
// place of the basic scanner.
type Scanner struct {
	primary dlp.DLPScanner // audimodal — authoritative
	gk      gk.Scanner     // Gatekeeper — shadow only
	log     Logger
	profile gk.ScanProfile
}

// New wraps primary with a Gatekeeper shadow. A nil primary or logger is a
// programming error; both are required. The Gatekeeper scanner is built with the
// default (full) profile.
func New(primary dlp.DLPScanner, log Logger) *Scanner {
	return &Scanner{
		primary: primary,
		gk:      gk.NewScanner(),
		log:     log,
		profile: gk.ProfileFull,
	}
}

var _ dlp.DLPScanner = (*Scanner)(nil)

// ScanContent runs the primary scanner (authoritative), shadows it with
// Gatekeeper, and returns the primary result unchanged.
func (s *Scanner) ScanContent(ctx context.Context, content string, config *types.ScanConfig) (*types.ScanResult, error) {
	res, err := s.primary.ScanContent(ctx, content, config)
	if err != nil {
		return res, err // primary error is the real error; don't shadow a failed scan
	}
	s.shadow(ctx, content, res)
	return res, nil
}

// ScanChunk mirrors ScanContent for the chunk path.
func (s *Scanner) ScanChunk(ctx context.Context, chunk *types.ChunkContent, config *types.ScanConfig) (*types.ScanResult, error) {
	res, err := s.primary.ScanChunk(ctx, chunk, config)
	if err != nil {
		return res, err
	}
	if chunk != nil {
		s.shadow(ctx, chunk.Content, res)
	}
	return res, nil
}

// GetSupportedPatterns and ValidateConfig delegate to the primary: the shadow
// must not change audimodal's advertised capabilities or config validation.
func (s *Scanner) GetSupportedPatterns() []types.PatternInfo { return s.primary.GetSupportedPatterns() }
func (s *Scanner) ValidateConfig(config *types.ScanConfig) error {
	return s.primary.ValidateConfig(config)
}

// shadow runs Gatekeeper on the same content and logs the per-type diff against
// the primary result. Best-effort: any error is logged and swallowed — a shadow
// scan can never fail or alter the ingest path. Runs synchronously so the diff
// is attributable to the scan; Gatekeeper's regexp/Hyperscan scan is fast
// relative to ingestion.
func (s *Scanner) shadow(ctx context.Context, content string, primary *types.ScanResult) {
	if s == nil || s.gk == nil || content == "" {
		return
	}
	cfg := gk.DefaultScanConfig()
	cfg.Profile = s.profile
	cfg.TrustTier = gk.TierExternal // ingested customer documents are untrusted

	gkRes, err := s.gk.ScanString(ctx, content, cfg)
	if err != nil {
		s.log.Printf("dlp-shadow: gatekeeper scan error (ignored): %v", err)
		return
	}

	d := diff(primary, gkRes)
	if d.empty() {
		return // both engines agree there's nothing here — no noise
	}
	s.log.Printf("dlp-shadow: %s", d.String())
}

// canonical normalizes a finding type from either engine to one shared
// vocabulary so counts compare correctly.
//
// The two engines do NOT emit the same strings, despite sharing a heritage:
//   - audimodal Finding.Type: "email", "ssn", "credit_card", "phone_number", …
//   - Gatekeeper PatternID:    "pii-email", "cred-aws-access-key", "injection-sql"
//
// So Gatekeeper's IDs carry a category prefix and use hyphens, and a few concepts
// diverge in spelling (Gatekeeper "phone"/"dob"/"mrn" vs audimodal
// "phone_number"/"date_of_birth"/"medical_record_number"). Running both through
// canonical() collapses those so the same real finding lands in the `both`
// bucket instead of being double-counted as gatekeeper_only AND audimodal_only.
// (This was a real bug the end-to-end test caught: the earlier code compared raw
// strings and reported every shared finding as a mismatch in both directions.)
func canonical(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"pii-", "cred-", "credential-", "injection-", "inj-"} {
		if strings.HasPrefix(s, p) {
			s = s[len(p):]
			break
		}
	}
	s = strings.ReplaceAll(s, "-", "_")
	switch s { // reconcile the handful of genuine spelling divergences
	case "phone":
		return "phone_number"
	case "dob":
		return "date_of_birth"
	case "mrn":
		return "medical_record_number"
	}
	return s
}

// diffResult tallies findings per canonical type. audimodal's Finding.Type and
// Gatekeeper's Finding.PatternID are both routed through canonical() first.
type diffResult struct {
	both           map[string]struct{ prim, gk int } // types both engines found
	gatekeeperOnly map[string]int                    // COVERAGE GAIN: gk found, audimodal missed
	primaryOnly    map[string]int                    // POTENTIAL REGRESSION: audimodal found, gk missed
}

func (d diffResult) empty() bool {
	return len(d.both) == 0 && len(d.gatekeeperOnly) == 0 && len(d.primaryOnly) == 0
}

func diff(primary *types.ScanResult, gkRes *gk.ScanResult) diffResult {
	prim := map[string]int{}
	if primary != nil {
		for i := range primary.Findings {
			prim[canonical(string(primary.Findings[i].Type))]++
		}
	}
	gkc := map[string]int{}
	if gkRes != nil {
		for i := range gkRes.Findings {
			gkc[canonical(gkRes.Findings[i].PatternID)]++
		}
	}

	d := diffResult{
		both:           map[string]struct{ prim, gk int }{},
		gatekeeperOnly: map[string]int{},
		primaryOnly:    map[string]int{},
	}
	for t, pc := range prim {
		if gc, ok := gkc[t]; ok {
			d.both[t] = struct{ prim, gk int }{pc, gc}
		} else {
			d.primaryOnly[t] = pc
		}
	}
	for t, gc := range gkc {
		if _, ok := prim[t]; !ok {
			d.gatekeeperOnly[t] = gc
		}
	}
	return d
}

// String renders the diff for a single log line. gatekeeper_only is the headline
// — it is the detection audimodal is missing today (secrets especially).
func (d diffResult) String() string {
	var b strings.Builder
	if len(d.gatekeeperOnly) > 0 {
		b.WriteString("gatekeeper_only=[")
		b.WriteString(fmtCounts(d.gatekeeperOnly))
		b.WriteString("] ")
	}
	if len(d.primaryOnly) > 0 {
		b.WriteString("audimodal_only=[")
		b.WriteString(fmtCounts(d.primaryOnly))
		b.WriteString("] ")
	}
	if len(d.both) > 0 {
		var parts []string
		for t, c := range d.both {
			p := t + ":" + itoa(c.prim) + "/" + itoa(c.gk)
			if c.prim != c.gk {
				p += "(mismatch)"
			}
			parts = append(parts, p)
		}
		sort.Strings(parts)
		b.WriteString("both(audimodal/gk)=[")
		b.WriteString(strings.Join(parts, " "))
		b.WriteString("]")
	}
	return strings.TrimSpace(b.String())
}

func fmtCounts(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for t, c := range m {
		parts = append(parts, t+":"+itoa(c))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// itoa avoids strconv for a tiny non-negative count.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
