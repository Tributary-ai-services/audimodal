# Compliance Standards - Detailed Reference

*Source: issues/compliance_standards.xlsx | Last Updated: January 2025*

This document provides comprehensive details for all compliance standards relevant to TAS/AudiModal, including enforcement bodies, audit frequencies, penalty ranges, and implementation guidance.

---

## Table of Contents

1. [Top 20 Core Standards](#top-20-core-standards)
2. [Currently Implemented Standards](#currently-implemented-standards)
3. [Industry-Specific Standards](#industry-specific-standards)
4. [Framework Mappings](#framework-mappings)
5. [TAS Priority Roadmap](#tas-priority-roadmap)
6. [Pattern Matchers Status](#pattern-matchers-status)

---

## Top 20 Core Standards

### Rank 1: SOC 2 (Service Organization Control Type 2)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Service Organization Control Type 2 |
| **Domain** | Security/Trust |
| **Enforcement Body** | AICPA |
| **Audit Frequency** | Annual |
| **Penalty Range** | $50K-$500K audit cost |
| **Why Essential** | De facto B2B SaaS requirement; requested in 90%+ enterprise procurement |
| **TAS Priority** | CRITICAL |
| **Implementation Effort** | Medium |
| **Status** | Planned |

### Rank 2: ISO 27001 (Information Security Management System)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Information Security Management System |
| **Domain** | Security |
| **Enforcement Body** | ISO/Accredited Certification Bodies |
| **Audit Frequency** | Annual surveillance, 3-year recertification |
| **Penalty Range** | $20K-$100K certification cost |
| **Why Essential** | Global gold standard; hard requirement for international deals |
| **TAS Priority** | CRITICAL |
| **Implementation Effort** | High |
| **Status** | Planned |

### Rank 3: GDPR (General Data Protection Regulation)

| Attribute | Value |
|-----------|-------|
| **Full Name** | General Data Protection Regulation |
| **Domain** | Privacy |
| **Enforcement Body** | EU Data Protection Authorities (DPAs) |
| **Audit Frequency** | Ongoing/complaint-driven |
| **Penalty Range** | Up to 4% global revenue or €20M |
| **Why Essential** | Applies to ANY company handling EU resident data |
| **TAS Priority** | CRITICAL |
| **Implementation Effort** | High |
| **Status** | ✅ **IMPLEMENTED** |
| **Rule IDs** | GDPR-001, GDPR-002 |
| **PII Types** | Email, Name, Address, Phone, SSN, DOB |

### Rank 4: PCI DSS (Payment Card Industry Data Security Standard)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Payment Card Industry Data Security Standard |
| **Domain** | Payment |
| **Enforcement Body** | PCI SSC / Card Brands |
| **Audit Frequency** | Annual (SAQ or QSA) |
| **Penalty Range** | Fines $5K-$100K/month |
| **Why Essential** | Mandatory if processing payment card data |
| **TAS Priority** | LOW (conditional on payment processing) |
| **Implementation Effort** | High |
| **Status** | ✅ **IMPLEMENTED** |
| **Rule IDs** | PCI-001 |
| **PII Types** | Credit Card Numbers |

### Rank 5: HIPAA (Health Insurance Portability and Accountability Act)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Health Insurance Portability and Accountability Act |
| **Domain** | Healthcare |
| **Enforcement Body** | HHS Office for Civil Rights (OCR) |
| **Audit Frequency** | Complaint-driven + random audits |
| **Penalty Range** | Up to $1.5M per violation category |
| **Why Essential** | Required for any entity handling PHI |
| **TAS Priority** | MEDIUM (conditional on healthcare clients) |
| **Implementation Effort** | High |
| **Status** | ✅ **IMPLEMENTED** |
| **Rule IDs** | HIPAA-001 |
| **PII Types** | SSN, DOB, Name, Email (as PHI) |

### Rank 6: SOX (Sarbanes-Oxley Act)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Sarbanes-Oxley Act |
| **Domain** | Financial |
| **Enforcement Body** | SEC / PCAOB |
| **Audit Frequency** | Annual |
| **Penalty Range** | Criminal penalties; executive certification liability |
| **Why Essential** | Mandatory for US public companies |
| **TAS Priority** | LOW (only if TAS goes public) |
| **Implementation Effort** | High |
| **Status** | Planned |

### Rank 7: CCPA/CPRA (California Consumer Privacy Act)

| Attribute | Value |
|-----------|-------|
| **Full Name** | California Consumer Privacy Act / Privacy Rights Act |
| **Domain** | Privacy |
| **Enforcement Body** | California AG / CPPA |
| **Audit Frequency** | Ongoing/complaint-driven |
| **Penalty Range** | $2,500-$7,500 per violation |
| **Why Essential** | Applies to most companies with CA customers |
| **TAS Priority** | HIGH |
| **Implementation Effort** | Medium |
| **Status** | ✅ **IMPLEMENTED** |
| **Rule IDs** | CCPA-001 |
| **PII Types** | Email, Name, Address, SSN, IP Address |

### Rank 8: NIST CSF (NIST Cybersecurity Framework)

| Attribute | Value |
|-----------|-------|
| **Full Name** | NIST Cybersecurity Framework |
| **Domain** | Security |
| **Enforcement Body** | NIST (voluntary) |
| **Audit Frequency** | Self-assessed or mapped |
| **Penalty Range** | N/A (framework only) |
| **Why Essential** | Most widely adopted security framework; contract reference |
| **TAS Priority** | CRITICAL |
| **Implementation Effort** | Low |
| **Status** | Planned |

### Rank 9: NIST 800-171 (Protecting CUI)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Protecting Controlled Unclassified Information |
| **Domain** | Security |
| **Enforcement Body** | DoD / NIST |
| **Audit Frequency** | Self-assessment + DIBCAC |
| **Penalty Range** | Contract termination; False Claims Act liability |
| **Why Essential** | Required for contractors handling CUI |
| **TAS Priority** | HIGH |
| **Implementation Effort** | High |
| **Status** | Planned |

### Rank 10: CMMC (Cybersecurity Maturity Model Certification)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Cybersecurity Maturity Model Certification |
| **Domain** | Defense |
| **Enforcement Body** | DoD / Cyber AB |
| **Audit Frequency** | Every 3 years (certified) |
| **Penalty Range** | Contract ineligibility |
| **Why Essential** | Mandatory for DoD contracts (phased 2025+) |
| **TAS Priority** | HIGH |
| **Implementation Effort** | Very High |
| **Status** | Planned |

### Rank 11: FedRAMP (Federal Risk and Authorization Management Program)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Federal Risk and Authorization Management Program |
| **Domain** | Government |
| **Enforcement Body** | GSA / FedRAMP PMO |
| **Audit Frequency** | Annual + continuous monitoring |
| **Penalty Range** | Market exclusion |
| **Why Essential** | Required to sell cloud services to federal agencies |
| **TAS Priority** | HIGH |
| **Implementation Effort** | Very High |
| **Status** | Planned |

### Rank 12: FISMA (Federal Information Security Management Act)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Federal Information Security Management Act |
| **Domain** | Government |
| **Enforcement Body** | OMB / DHS / NIST |
| **Audit Frequency** | Annual |
| **Penalty Range** | Funding/contract impact |
| **Why Essential** | Required for federal agency systems |
| **TAS Priority** | MEDIUM |
| **Implementation Effort** | High |
| **Status** | Planned |

### Rank 13: SOC 1 (Service Organization Control Type 1)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Service Organization Control Type 1 |
| **Domain** | Financial |
| **Enforcement Body** | AICPA |
| **Audit Frequency** | Annual |
| **Penalty Range** | $30K-$200K audit cost |
| **Why Essential** | Required when services impact client financial reporting |
| **TAS Priority** | LOW |
| **Implementation Effort** | Medium |
| **Status** | Planned |

### Rank 14: GLBA (Gramm-Leach-Bliley Act)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Gramm-Leach-Bliley Act |
| **Domain** | Financial |
| **Enforcement Body** | FTC / OCC / CFPB |
| **Audit Frequency** | Ongoing |
| **Penalty Range** | Up to $100K per violation |
| **Why Essential** | Required for financial institutions |
| **TAS Priority** | LOW |
| **Implementation Effort** | Medium |
| **Status** | Planned |

### Rank 15: HITRUST CSF (Health Information Trust Alliance CSF)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Health Information Trust Alliance Common Security Framework |
| **Domain** | Healthcare+ |
| **Enforcement Body** | HITRUST Alliance |
| **Audit Frequency** | Annual (certified) |
| **Penalty Range** | $50K-$200K certification cost |
| **Why Essential** | Increasingly required by healthcare enterprises |
| **TAS Priority** | MEDIUM |
| **Implementation Effort** | Very High |
| **Status** | Planned |

### Rank 16: ISO 27701 (Privacy Information Management)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Privacy Information Management |
| **Domain** | Privacy |
| **Enforcement Body** | ISO/Accredited Certification Bodies |
| **Audit Frequency** | Annual surveillance |
| **Penalty Range** | $15K-$50K additional to ISO 27001 |
| **Why Essential** | Privacy extension to ISO 27001; GDPR demonstration |
| **TAS Priority** | HIGH |
| **Implementation Effort** | Medium |
| **Status** | Planned |

### Rank 17: NIST 800-53 (Security and Privacy Controls)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Security and Privacy Controls |
| **Domain** | Security |
| **Enforcement Body** | NIST / Federal agencies |
| **Audit Frequency** | Per system ATO cycle |
| **Penalty Range** | N/A (framework) |
| **Why Essential** | Foundation for federal security; cross-industry reference |
| **TAS Priority** | HIGH |
| **Implementation Effort** | High |
| **Status** | Planned |

### Rank 18: CSA STAR (Cloud Security Alliance STAR)

| Attribute | Value |
|-----------|-------|
| **Full Name** | Cloud Security Alliance Security Trust Assurance Registry |
| **Domain** | Cloud |
| **Enforcement Body** | Cloud Security Alliance |
| **Audit Frequency** | Annual |
| **Penalty Range** | $5K-$30K assessment cost |
| **Why Essential** | Expected for cloud providers |
| **TAS Priority** | HIGH |
| **Implementation Effort** | Low |
| **Status** | Planned |

### Rank 19: EU AI Act (EU Artificial Intelligence Act)

| Attribute | Value |
|-----------|-------|
| **Full Name** | EU Artificial Intelligence Act |
| **Domain** | AI |
| **Enforcement Body** | EU AI Office / Member States |
| **Audit Frequency** | Ongoing (effective 2025-2027) |
| **Penalty Range** | Up to 7% global revenue or €35M |
| **Why Essential** | Mandatory for AI systems in EU market |
| **TAS Priority** | CRITICAL |
| **Implementation Effort** | High |
| **Status** | Planned |

### Rank 20: NIST AI RMF (AI Risk Management Framework)

| Attribute | Value |
|-----------|-------|
| **Full Name** | AI Risk Management Framework |
| **Domain** | AI |
| **Enforcement Body** | NIST (voluntary) |
| **Audit Frequency** | Self-assessed |
| **Penalty Range** | N/A (framework) |
| **Why Essential** | Emerging standard for AI governance |
| **TAS Priority** | CRITICAL |
| **Implementation Effort** | Low |
| **Status** | Planned |

---

## Currently Implemented Standards

### Implementation Summary

| Standard | Version | Location | Rule Count | Last Updated |
|----------|---------|----------|------------|--------------|
| GDPR | 1.0 | `pkg/dlp/compliance/checker.go` | 2 rules | Jan 2025 |
| HIPAA | 1.0 | `pkg/dlp/compliance/checker.go` | 1 rule | Jan 2025 |
| PCI-DSS | 1.0 | `pkg/dlp/compliance/checker.go` | 1 rule | Jan 2025 |
| CCPA | 1.0 | `pkg/dlp/compliance/checker.go` | 1 rule | Jan 2025 |

### GDPR Implementation Details

```
Rule: GDPR-001 - Personal Data Detection
  Description: Detect personal data that requires GDPR compliance
  PII Types: Email, Name, Address, Phone Number
  Severity: High
  Validator: validateGDPRPersonalData()

Rule: GDPR-002 - Special Category Data
  Description: Detect special category personal data requiring enhanced protection
  PII Types: Date of Birth, SSN
  Severity: Critical
  Validator: validateGDPRSpecialCategory()
```

### HIPAA Implementation Details

```
Rule: HIPAA-001 - Protected Health Information
  Description: Detect PHI that requires HIPAA compliance
  PII Types: SSN, Date of Birth, Name, Email
  Severity: Critical
  Validator: validateHIPAAPHI()
```

### PCI-DSS Implementation Details

```
Rule: PCI-001 - Cardholder Data
  Description: Detect cardholder data requiring PCI DSS compliance
  PII Types: Credit Card
  Severity: Critical
  Validator: validatePCICardholderData()
```

### CCPA Implementation Details

```
Rule: CCPA-001 - Personal Information
  Description: Detect personal information under CCPA
  PII Types: Email, Name, Address, SSN, IP Address
  Severity: High
  Validator: validateCCPAPersonalInfo()
```

---

## Industry-Specific Standards

### Financial Services

| Standard | Full Name | Jurisdiction | Enforcement Body | TAS Relevance |
|----------|-----------|--------------|------------------|---------------|
| Basel III/IV | Basel Capital Accords | Global Banking | BIS / National regulators | None |
| MiFID II | Markets in Financial Instruments Directive | EU | ESMA / National regulators | None |
| DORA | Digital Operational Resilience Act | EU | EU / ESAs | Low - if financial clients |
| SWIFT CSP | SWIFT Customer Security Programme | Global | SWIFT | None |
| NACHA | ACH Network Rules | US | NACHA | None |

### Healthcare

| Standard | Full Name | Jurisdiction | Enforcement Body | TAS Relevance |
|----------|-----------|--------------|------------------|---------------|
| HITECH | Health IT for Economic and Clinical Health | US | HHS OCR | Medium - extends HIPAA |
| FDA 21 CFR Part 11 | Electronic Records/Signatures | US | FDA | Low |

### Government/Defense

| Standard | Full Name | Jurisdiction | Enforcement Body | TAS Relevance |
|----------|-----------|--------------|------------------|---------------|
| ITAR | International Traffic in Arms Regulations | US | State Dept/DDTC | Low |
| EAR | Export Administration Regulations | US | Commerce/BIS | Medium - AI export |
| DFARS | Defense Federal Acquisition Regulation | US | DoD | High - if DoD target |
| StateRAMP | State Risk Authorization Management | US States | StateRAMP PMO | Medium |
| TX-RAMP | Texas Risk Authorization Management | Texas | DIR | Low |
| CJIS | Criminal Justice Information Services | US | FBI | Low |

### International Privacy Laws

| Standard | Full Name | Jurisdiction | TAS Relevance |
|----------|-----------|--------------|---------------|
| LGPD | Lei Geral de Proteção de Dados | Brazil | Medium - if LATAM |
| PIPEDA | Personal Information Protection Act | Canada | Medium - if Canada |
| POPIA | Protection of Personal Information Act | South Africa | Low |
| PDPA (SG) | Personal Data Protection Act | Singapore | Medium - if APAC |
| PDPA (TH) | Personal Data Protection Act | Thailand | Low |
| APPI | Act on Protection of Personal Information | Japan | Medium - if APAC |
| Privacy Act 1988 | Australian Privacy Act | Australia | Medium - if APAC |
| VCDPA | Virginia Consumer Data Protection Act | Virginia | Low |
| CPA | Colorado Privacy Act | Colorado | Low |
| CTDPA | Connecticut Data Privacy Act | Connecticut | Low |

### Cloud Security Standards

| Standard | Full Name | TAS Relevance |
|----------|-----------|---------------|
| ISO 27017 | Cloud Security Controls | High |
| ISO 27018 | Cloud PII Protection | High |
| ISO 22301 | Business Continuity Management | Medium |
| ISO 31000 | Risk Management | Medium |
| Cyber Essentials | UK Cyber Essentials | Medium - if UK |
| IRAP | InfoSec Registered Assessors Program | Low - Australia |
| C5 | Cloud Computing Compliance Catalogue | Medium - if Germany |
| MTCS | Multi-Tier Cloud Security | Medium - if APAC |

### AI-Specific Standards

| Standard | Full Name | Jurisdiction | TAS Relevance |
|----------|-----------|--------------|---------------|
| ISO 42001 | AI Management System Standard | Global | HIGH |
| NYC LL 144 | Automated Employment Decision Tools | NYC | Medium |
| Colorado AI Act | SB 24-205 | Colorado | Medium |

---

## Framework Mappings

Understanding how frameworks map to each other helps optimize compliance efforts:

### High-Coverage Mappings

| If You Have | You Also Largely Cover | Coverage % |
|-------------|----------------------|------------|
| HITRUST CSF | HIPAA, SOC 2, NIST CSF, ISO 27001 | 80-100% |
| FedRAMP | NIST 800-53, FISMA | 100% |
| CMMC Level 2 | NIST 800-171 | 100% |
| ISO 27701 | GDPR, CCPA | 70-80% |
| SOC 2 | ISO 27001 (partial), NIST CSF (partial) | 60-70% |

### Recommended Compliance Paths

**Path A: Enterprise SaaS (Most Common)**
```
SOC 2 Type I → SOC 2 Type II → ISO 27001 → ISO 27701
Timeline: 6 months → 12 months → 18 months → 24 months
```

**Path B: Healthcare Focus**
```
HIPAA → SOC 2 → HITRUST CSF
Timeline: 6 months → 12 months → 24 months
```

**Path C: Federal Government**
```
NIST CSF → NIST 800-171 → FedRAMP Ready → FedRAMP Authorized
Timeline: 3 months → 12 months → 18 months → 24+ months
```

**Path D: AI Platform**
```
NIST AI RMF → SOC 2 → EU AI Act Prep → ISO 42001
Timeline: 3 months → 12 months → 18 months → 24 months
```

---

## TAS Priority Roadmap

### Phase Timeline

| Phase | Timeline | Standards | Effort |
|-------|----------|-----------|--------|
| Foundation | Months 1-6 | SOC 2 Type I, NIST CSF, NIST AI RMF, GDPR✅, CCPA✅ | Medium |
| Credibility | Months 6-12 | SOC 2 Type II, ISO 27001, CSA STAR L1, ISO 27701 | High |
| AI Leadership | Months 12-18 | EU AI Act Prep, ISO 42001, CSA STAR L2 | High |
| Government | Months 18-24 | NIST 800-171, FedRAMP Ready, StateRAMP | Very High |
| Expansion | Months 24-36 | CMMC Level 2, FedRAMP Moderate, HITRUST | Very High |

### Conditional Standards

These standards should be implemented based on specific business needs:

| Standard | Trigger Condition | Priority When Triggered |
|----------|-------------------|------------------------|
| HIPAA | Healthcare clients handling PHI | HIGH |
| PCI DSS | Processing payment card data | HIGH |
| SOX | TAS goes public | HIGH |
| LGPD | Latin American expansion | MEDIUM |
| PIPEDA | Canadian expansion | MEDIUM |

---

## Pattern Matchers Status

### Currently Implemented Matchers

| PII Type | Matcher | Status | Risk Level | Confidence Range |
|----------|---------|--------|------------|------------------|
| SSN | `SSNMatcher` | ✅ Implemented | Critical (0.9) | 0.3-0.9 |
| Credit Card | `CreditCardMatcher` | ✅ Implemented | Critical (0.9) | 0.0-0.9 |
| Email | `EmailMatcher` | ✅ Implemented | Low (0.4) | 0.0-0.9 |
| Phone Number | `PhoneNumberMatcher` | ✅ Implemented | Medium (0.5) | 0.7-0.9 |
| IP Address | `IPAddressMatcher` | ✅ Implemented | Low (0.3) | 0.0-0.8 |

### Planned Matchers (Types Defined, No Matcher)

| PII Type | Risk Level | Priority | Target Compliance |
|----------|------------|----------|-------------------|
| Bank Account | High (0.8) | Medium | GLBA, SOX |
| Passport | High (0.8) | Medium | GDPR, Immigration |
| Driver's License | High (0.7) | Medium | CCPA, KYC |
| Date of Birth | High (0.7) | High | GDPR, HIPAA |
| Address | Medium (0.6) | High | GDPR, CCPA |
| Name | Medium (0.5) | High | GDPR, HIPAA, CCPA |

### Validation Rules by Matcher

**SSN Matcher Validation:**
- Must be exactly 9 digits
- Cannot start with 000, 666, or 9xx
- Middle 2 digits cannot be 00
- Last 4 digits cannot be 0000

**Credit Card Matcher Validation:**
- Luhn algorithm checksum validation
- Supports: Visa, Mastercard, Amex, Discover
- Length: 13-19 digits depending on card type

**Email Matcher Validation:**
- RFC-compliant email format
- Must contain @ and valid domain

**Phone Number Matcher Validation:**
- US formats: (xxx) xxx-xxxx, xxx-xxx-xxxx, xxx.xxx.xxxx
- International: +1-xxx-xxx-xxxx

**IP Address Matcher Validation:**
- IPv4 format: x.x.x.x where each octet is 0-255

---

## Testing Requirements

### Compliance Test Categories

1. **Positive Detection Tests** - Verify PII is correctly detected
2. **Negative Detection Tests** - Verify false positives are minimized
3. **Edge Case Tests** - Boundary conditions and malformed data
4. **Integration Tests** - Full pipeline with PDF/TXT processing
5. **Performance Tests** - Benchmark speed and memory usage

### Test Data Requirements

| Category | Test Files Needed | PII Types |
|----------|------------------|-----------|
| GDPR | 2 | Email, Name, Address, Phone, SSN, DOB |
| HIPAA | 1 | SSN, DOB, Name, Email |
| PCI-DSS | 1 | Credit Card |
| CCPA | 1 | Email, Name, Address, SSN, IP |
| Edge Cases | 3 | All types - malformed/partial |
| Negative | 1 | Clean document |

### Coverage Targets

| Component | Target Coverage |
|-----------|----------------|
| Pattern Matchers | 90% |
| Compliance Checker | 85% |
| Integration Tests | 80% |

---

## References

- [SOC 2 Trust Services Criteria](https://www.aicpa.org/interestareas/frc/assuranceadvisoryservices/sorhome.html)
- [ISO 27001:2022 Standard](https://www.iso.org/standard/27001)
- [GDPR Official Text](https://gdpr.eu/)
- [HIPAA Regulations](https://www.hhs.gov/hipaa/)
- [PCI DSS v4.0](https://www.pcisecuritystandards.org/)
- [CCPA/CPRA Text](https://oag.ca.gov/privacy/ccpa)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [EU AI Act](https://artificialintelligenceact.eu/)
- [NIST AI RMF](https://www.nist.gov/itl/ai-risk-management-framework)
