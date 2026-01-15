// Package middleware provides HTTP middleware components
package middleware

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// TCF v2 Purpose IDs (IAB specification)
const (
	PurposeStorageAccess          = 1  // Store and/or access information on a device
	PurposeBasicAds               = 2  // Select basic ads
	PurposePersonalizedAdsProfile = 3  // Create a personalised ads profile
	PurposePersonalizedAds        = 4  // Select personalised ads
	PurposeContentProfile         = 5  // Create a personalised content profile
	PurposePersonalizedContent    = 6  // Select personalised content
	PurposeMeasureAdPerformance   = 7  // Measure ad performance
	PurposeMeasureContent         = 8  // Measure content performance
	PurposeMarketResearch         = 9  // Apply market research
	PurposeProductDevelopment     = 10 // Develop and improve products
)

// Required purposes for programmatic advertising
var RequiredPurposes = []int{
	PurposeStorageAccess,        // Required for cookies
	PurposeBasicAds,             // Required for ad selection
	PurposeMeasureAdPerformance, // Required for reporting
}

// PrivacyRegulation represents different privacy regulations
type PrivacyRegulation string

const (
	RegulationGDPR   PrivacyRegulation = "GDPR"   // EU/EEA - TCF v2 consent
	RegulationCCPA   PrivacyRegulation = "CCPA"   // California - US Privacy String
	RegulationVCDPA  PrivacyRegulation = "VCDPA"  // Virginia - US Privacy String
	RegulationCPA    PrivacyRegulation = "CPA"    // Colorado - US Privacy String
	RegulationCTDPA  PrivacyRegulation = "CTDPA"  // Connecticut - US Privacy String
	RegulationUCPA   PrivacyRegulation = "UCPA"   // Utah - US Privacy String
	RegulationLGPD   PrivacyRegulation = "LGPD"   // Brazil
	RegulationPIPEDA PrivacyRegulation = "PIPEDA" // Canada
	RegulationPDPA   PrivacyRegulation = "PDPA"   // Singapore
	RegulationNone   PrivacyRegulation = "NONE"   // No applicable regulation
)

// GDPR Countries (EU/EEA + UK) - ISO 3166-1 alpha-3 codes
var gdprCountries = map[string]bool{
	"AUT": true, // Austria
	"BEL": true, // Belgium
	"BGR": true, // Bulgaria
	"HRV": true, // Croatia
	"CYP": true, // Cyprus
	"CZE": true, // Czech Republic
	"DNK": true, // Denmark
	"EST": true, // Estonia
	"FIN": true, // Finland
	"FRA": true, // France
	"DEU": true, // Germany
	"GRC": true, // Greece
	"HUN": true, // Hungary
	"IRL": true, // Ireland
	"ITA": true, // Italy
	"LVA": true, // Latvia
	"LTU": true, // Lithuania
	"LUX": true, // Luxembourg
	"MLT": true, // Malta
	"NLD": true, // Netherlands
	"POL": true, // Poland
	"PRT": true, // Portugal
	"ROU": true, // Romania
	"SVK": true, // Slovakia
	"SVN": true, // Slovenia
	"ESP": true, // Spain
	"SWE": true, // Sweden
	"GBR": true, // United Kingdom
	"ISL": true, // Iceland (EEA)
	"LIE": true, // Liechtenstein (EEA)
	"NOR": true, // Norway (EEA)
}

// US States with privacy laws - Two-letter state codes
var usPrivacyStates = map[string]PrivacyRegulation{
	"CA": RegulationCCPA,  // California - CCPA
	"VA": RegulationVCDPA, // Virginia - VCDPA
	"CO": RegulationCPA,   // Colorado - CPA
	"CT": RegulationCTDPA, // Connecticut - CTDPA
	"UT": RegulationUCPA,  // Utah - UCPA
}

// PrivacyConfig configures the privacy middleware behavior
type PrivacyConfig struct {
	// EnforceGDPR requires valid consent when regs.gdpr=1
	EnforceGDPR bool
	// EnforceCOPPA blocks requests with COPPA=1 (child-directed)
	EnforceCOPPA bool
	// EnforceCCPA blocks/strips data when user opts out
	EnforceCCPA bool
	// GeoEnforcement validates consent strings match user's geographic location
	// When enabled, verifies EU users have GDPR consent, CA users have CCPA, etc.
	GeoEnforcement bool
	// RequiredPurposes - TCF purposes required for processing (default: 1, 2, 7)
	RequiredPurposes []int
	// StrictMode - if true, reject invalid consent strings; if false, strip PII
	StrictMode bool
	// AnonymizeIP - P2-2: if true, anonymize IP addresses when GDPR applies
	AnonymizeIP bool
}

// DefaultPrivacyConfig returns a sensible default config
// It reads from environment variables if set:
//   - PBS_ENFORCE_GDPR: "true" or "false" (default: true)
//   - PBS_ENFORCE_COPPA: "true" or "false" (default: true)
//   - PBS_ENFORCE_CCPA: "true" or "false" (default: true)
//   - PBS_GEO_ENFORCEMENT: "true" or "false" (default: true)
//   - PBS_PRIVACY_STRICT_MODE: "true" or "false" (default: true)
//   - PBS_ANONYMIZE_IP: "true" or "false" (default: true)
func DefaultPrivacyConfig() PrivacyConfig {
	return PrivacyConfig{
		EnforceGDPR:      getEnvBool("PBS_ENFORCE_GDPR", true),
		EnforceCOPPA:     getEnvBool("PBS_ENFORCE_COPPA", true),
		EnforceCCPA:      getEnvBool("PBS_ENFORCE_CCPA", true),
		GeoEnforcement:   getEnvBool("PBS_GEO_ENFORCEMENT", true),
		RequiredPurposes: RequiredPurposes,
		StrictMode:       getEnvBool("PBS_PRIVACY_STRICT_MODE", true),
		AnonymizeIP:      getEnvBool("PBS_ANONYMIZE_IP", true),
	}
}

// getEnvBool reads a boolean from environment variable with a default
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return strings.ToLower(val) == "true" || val == "1"
}

// PrivacyMiddleware enforces privacy regulations before auction execution
type PrivacyMiddleware struct {
	config PrivacyConfig
	next   http.Handler
}

// NewPrivacyMiddleware creates a new privacy enforcement middleware
func NewPrivacyMiddleware(config PrivacyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &PrivacyMiddleware{
			config: config,
			next:   next,
		}
	}
}

// ServeHTTP implements the http.Handler interface
func (m *PrivacyMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only process POST requests to auction endpoint
	if r.Method != http.MethodPost {
		m.next.ServeHTTP(w, r)
		return
	}

	// Read and parse the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.next.ServeHTTP(w, r)
		return
	}
	r.Body.Close()

	var bidRequest openrtb.BidRequest
	if err := json.Unmarshal(body, &bidRequest); err != nil {
		// Let the handler deal with invalid JSON
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		m.next.ServeHTTP(w, r)
		return
	}

	// Check privacy compliance
	violation := m.checkPrivacyCompliance(&bidRequest)
	if violation != nil {
		logger.Log.Warn().
			Str("request_id", bidRequest.ID).
			Str("violation", violation.Reason).
			Str("regulation", violation.Regulation).
			Msg("Privacy compliance violation - blocking request")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"error":      "Privacy compliance violation",
			"reason":     violation.Reason,
			"regulation": violation.Regulation,
			"nbr":        violation.NoBidReason,
		})
		return
	}

	// P2-2: Anonymize IP addresses when GDPR applies and anonymization is enabled
	requestModified := false
	if m.config.AnonymizeIP && m.isGDPRApplicable(&bidRequest) {
		m.anonymizeRequestIPs(&bidRequest)
		requestModified = true
	}

	// Re-create request body for downstream handler
	if requestModified {
		// Re-marshal the modified request
		modifiedBody, err := json.Marshal(&bidRequest)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to marshal modified request after IP anonymization")
			r.Body = io.NopCloser(strings.NewReader(string(body)))
		} else {
			r.Body = io.NopCloser(strings.NewReader(string(modifiedBody)))
			r.ContentLength = int64(len(modifiedBody))
		}
	} else {
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}
	m.next.ServeHTTP(w, r)
}

// PrivacyViolation describes a privacy compliance failure
type PrivacyViolation struct {
	Regulation  string              // "GDPR", "COPPA", "CCPA"
	Reason      string              // Human-readable reason
	NoBidReason openrtb.NoBidReason // P2-7: Using consolidated type from openrtb
}

// detectApplicableRegulation determines which privacy regulation applies based on user geo
func (m *PrivacyMiddleware) detectApplicableRegulation(req *openrtb.BidRequest) PrivacyRegulation {
	if req.Device == nil || req.Device.Geo == nil {
		return RegulationNone
	}

	geo := req.Device.Geo

	// Check GDPR countries (EU/EEA + UK)
	if geo.Country != "" && gdprCountries[geo.Country] {
		return RegulationGDPR
	}

	// Check US state privacy laws
	if geo.Country == "USA" && geo.Region != "" {
		if regulation, exists := usPrivacyStates[geo.Region]; exists {
			return regulation
		}
		// Other US states without specific laws
		return RegulationNone
	}

	// Check other countries with privacy laws
	switch geo.Country {
	case "BRA": // Brazil
		return RegulationLGPD
	case "CAN": // Canada
		return RegulationPIPEDA
	case "SGP": // Singapore
		return RegulationPDPA
	}

	return RegulationNone
}

// validateGeoConsent checks if the request has appropriate consent for the detected geo
func (m *PrivacyMiddleware) validateGeoConsent(req *openrtb.BidRequest) *PrivacyViolation {
	if !m.config.GeoEnforcement {
		return nil // Geo enforcement disabled
	}

	detectedReg := m.detectApplicableRegulation(req)

	// If no specific regulation applies by geo, no geo-based enforcement needed
	if detectedReg == RegulationNone {
		return nil
	}

	// Check if request has appropriate consent signals for detected regulation
	switch detectedReg {
	case RegulationGDPR:
		// EU user should have GDPR flag and TCF consent
		if req.Regs == nil || req.Regs.GDPR == nil || *req.Regs.GDPR != 1 {
			logger.Log.Warn().
				Str("request_id", req.ID).
				Str("country", req.Device.Geo.Country).
				Msg("EU user detected but no GDPR flag set")
			return &PrivacyViolation{
				Regulation:  "GDPR",
				Reason:      "User in EU/EEA but GDPR consent not provided (regs.gdpr must be 1)",
				NoBidReason: openrtb.NoBidAdsNotAllowed,
			}
		}

	case RegulationCCPA, RegulationVCDPA, RegulationCPA, RegulationCTDPA, RegulationUCPA:
		// US state with privacy law should have US Privacy String
		if req.Regs == nil || req.Regs.USPrivacy == "" {
			logger.Log.Warn().
				Str("request_id", req.ID).
				Str("country", req.Device.Geo.Country).
				Str("region", req.Device.Geo.Region).
				Str("regulation", string(detectedReg)).
				Msg("US privacy state detected but no US Privacy String provided")
			return &PrivacyViolation{
				Regulation:  string(detectedReg),
				Reason:      "User in US privacy state but consent string not provided (regs.us_privacy required)",
				NoBidReason: openrtb.NoBidAdsNotAllowed,
			}
		}

	case RegulationLGPD, RegulationPIPEDA, RegulationPDPA:
		// Other regulations - log but don't block (not fully implemented yet)
		logger.Log.Info().
			Str("request_id", req.ID).
			Str("country", req.Device.Geo.Country).
			Str("regulation", string(detectedReg)).
			Msg("Privacy regulation detected - enforcement not yet implemented")
	}

	return nil
}

// checkPrivacyCompliance verifies the request meets privacy requirements
func (m *PrivacyMiddleware) checkPrivacyCompliance(req *openrtb.BidRequest) *PrivacyViolation {
	// First check geo-based consent requirements
	if violation := m.validateGeoConsent(req); violation != nil {
		return violation
	}
	// Check COPPA compliance
	if m.config.EnforceCOPPA && req.Regs != nil && req.Regs.COPPA == 1 {
		// COPPA requests require special handling - we block by default
		// Production systems might strip identifiers instead
		return &PrivacyViolation{
			Regulation:  "COPPA",
			Reason:      "Child-directed content requires COPPA-compliant handling",
			NoBidReason: openrtb.NoBidAdsNotAllowed,
		}
	}

	// Check GDPR compliance
	if m.config.EnforceGDPR && m.isGDPRApplicable(req) {
		violation := m.validateGDPRConsent(req)
		if violation != nil {
			return violation
		}
	}

	// Check US Privacy (CCPA) - P0: Enforce opt-out
	if req.Regs != nil && req.Regs.USPrivacy != "" {
		violation := m.checkCCPACompliance(req.ID, req.Regs.USPrivacy)
		if violation != nil {
			return violation
		}
	}

	return nil
}

// isGDPRApplicable checks if GDPR applies to this request
func (m *PrivacyMiddleware) isGDPRApplicable(req *openrtb.BidRequest) bool {
	if req.Regs == nil {
		return false
	}
	// GDPR applies if regs.gdpr == 1
	return req.Regs.GDPR != nil && *req.Regs.GDPR == 1
}

// validateGDPRConsent validates the TCF consent string and purpose consents
func (m *PrivacyMiddleware) validateGDPRConsent(req *openrtb.BidRequest) *PrivacyViolation {
	// Get consent string
	consentString := ""
	if req.User != nil {
		consentString = req.User.Consent
	}

	// No consent string when GDPR applies = violation
	if consentString == "" {
		return &PrivacyViolation{
			Regulation:  "GDPR",
			Reason:      "Missing consent string when GDPR applies (regs.gdpr=1)",
			NoBidReason: openrtb.NoBidAdsNotAllowed,
		}
	}

	// Parse and validate the consent string
	tcfData, err := m.parseTCFv2String(consentString)
	if err != nil {
		return &PrivacyViolation{
			Regulation:  "GDPR",
			Reason:      "Invalid TCF v2 consent string: " + err.Error(),
			NoBidReason: openrtb.NoBidInvalidRequest,
		}
	}

	// Check required purposes have consent (only in StrictMode)
	if m.config.StrictMode && len(m.config.RequiredPurposes) > 0 {
		missingPurposes := m.checkPurposeConsents(tcfData, m.config.RequiredPurposes)
		if len(missingPurposes) > 0 {
			logger.Log.Info().
				Str("request_id", req.ID).
				Ints("missing_purposes", missingPurposes).
				Msg("Missing required purpose consents")
			return &PrivacyViolation{
				Regulation:  "GDPR",
				Reason:      "Missing consent for required purposes",
				NoBidReason: openrtb.NoBidAdsNotAllowed,
			}
		}
	}

	return nil
}

// TCFv2Data holds parsed TCF v2 consent data
type TCFv2Data struct {
	Version           int
	Created           int64
	LastUpdated       int64
	CmpID             int
	CmpVersion        int
	ConsentScreen     int
	ConsentLanguage   string
	VendorListVersion int
	PurposeConsents   []bool // Indexed by purpose ID (1-based in spec, 0-based here)
	VendorConsents    map[int]bool
}

// parseTCFv2String parses a TCF v2 consent string and extracts purpose consents
func (m *PrivacyMiddleware) parseTCFv2String(consent string) (*TCFv2Data, error) {
	if consent == "" {
		return nil, nil
	}

	// Minimum reasonable length for a TCF v2 string
	if len(consent) < 20 {
		return nil, errInvalidTCFLength
	}

	// Try base64url decoding first, then standard base64
	decoded, err := base64.RawURLEncoding.DecodeString(consent)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(consent)
		if err != nil {
			return nil, errInvalidTCFEncoding
		}
	}

	// Minimum decoded length
	if len(decoded) < 15 {
		return nil, errInvalidTCFLength
	}

	data := &TCFv2Data{
		PurposeConsents: make([]bool, 24), // 24 purposes in TCF v2
		VendorConsents:  make(map[int]bool),
	}

	// Parse using bit reader
	reader := newBitReader(decoded)

	// Version (6 bits)
	data.Version = reader.readInt(6)
	if data.Version != 2 && data.Version != 1 {
		return nil, errInvalidTCFVersion
	}

	if data.Version == 1 {
		logger.Log.Warn().Msg("TCF v1 consent string - consider upgrading to v2")
		// For v1, we can't parse purposes the same way, so just accept it
		return data, nil
	}

	// For TCF v2, continue parsing
	// Created (36 bits - deciseconds since Jan 1, 2000)
	data.Created = int64(reader.readInt(36))
	// LastUpdated (36 bits)
	data.LastUpdated = int64(reader.readInt(36))
	// CmpId (12 bits)
	data.CmpID = reader.readInt(12)
	// CmpVersion (12 bits)
	data.CmpVersion = reader.readInt(12)
	// ConsentScreen (6 bits)
	data.ConsentScreen = reader.readInt(6)
	// ConsentLanguage (12 bits - 2 chars)
	lang1 := byte(reader.readInt(6)) + 'a'
	lang2 := byte(reader.readInt(6)) + 'a'
	data.ConsentLanguage = string([]byte{lang1, lang2})
	// VendorListVersion (12 bits)
	data.VendorListVersion = reader.readInt(12)
	// TcfPolicyVersion (6 bits) - skip
	reader.readInt(6)
	// IsServiceSpecific (1 bit) - skip
	reader.readInt(1)
	// UseNonStandardStacks (1 bit) - skip
	reader.readInt(1)
	// SpecialFeatureOptIns (12 bits) - skip
	reader.readInt(12)

	// Purpose consents (24 bits - one for each purpose)
	for i := 0; i < 24; i++ {
		data.PurposeConsents[i] = reader.readBool()
	}

	// Purpose legitimate interests (24 bits) - skip for now
	reader.readInt(24)

	// Special purposes (12 bits) - skip
	reader.readInt(12)

	// Features (24 bits) - skip
	reader.readInt(24)

	// Special features (12 bits) - skip
	reader.readInt(12)

	// Purpose legitimate interest (24 bits) - skip
	reader.readInt(24)

	// NumEntries for publisher purposes (12 bits) - skip
	reader.readInt(12)

	// Parse MaxVendorId for the consent section (16 bits)
	maxVendorID := reader.readInt(16)

	// IsRangeEncoding (1 bit)
	isRangeEncoding := reader.readBool()

	if isRangeEncoding {
		// Range encoding: parse vendor consent ranges
		numEntries := reader.readInt(12)
		for i := 0; i < numEntries; i++ {
			isRange := reader.readBool()
			if isRange {
				// Range: start and end vendor IDs
				startVendorID := reader.readInt(16)
				endVendorID := reader.readInt(16)
				// Mark all vendors in range as consented
				for vendorID := startVendorID; vendorID <= endVendorID; vendorID++ {
					data.VendorConsents[vendorID] = true
				}
			} else {
				// Single vendor ID
				vendorID := reader.readInt(16)
				data.VendorConsents[vendorID] = true
			}
		}
	} else {
		// BitField encoding: one bit per vendor up to MaxVendorId
		for vendorID := 1; vendorID <= maxVendorID; vendorID++ {
			if reader.readBool() {
				data.VendorConsents[vendorID] = true
			}
		}
	}

	return data, nil
}

// checkPurposeConsents verifies required purposes have consent
func (m *PrivacyMiddleware) checkPurposeConsents(data *TCFv2Data, required []int) []int {
	if data == nil {
		return required
	}

	var missing []int
	for _, purpose := range required {
		// Purpose IDs are 1-based in spec, array is 0-based
		idx := purpose - 1
		if idx < 0 || idx >= len(data.PurposeConsents) || !data.PurposeConsents[idx] {
			missing = append(missing, purpose)
		}
	}
	return missing
}

// CheckVendorConsent checks if a specific vendor (GVL ID) has consent
// Returns true if the vendor has consent, false otherwise
func (m *PrivacyMiddleware) CheckVendorConsent(consentString string, gvlID int) bool {
	if consentString == "" || gvlID <= 0 {
		return false
	}

	// Parse TCF consent string
	tcfData, err := m.parseTCFv2String(consentString)
	if err != nil {
		logger.Log.Debug().
			Err(err).
			Int("gvl_id", gvlID).
			Msg("Failed to parse TCF string for vendor consent check")
		return false
	}

	if tcfData == nil || tcfData.VendorConsents == nil {
		return false
	}

	// Check if vendor has consent
	hasConsent, exists := tcfData.VendorConsents[gvlID]
	return exists && hasConsent
}

// CheckVendorConsents checks multiple vendor IDs and returns which ones are missing consent
// Returns a map of vendor ID -> has consent
func (m *PrivacyMiddleware) CheckVendorConsents(consentString string, gvlIDs []int) map[int]bool {
	result := make(map[int]bool)

	if consentString == "" {
		// No consent string - all vendors lack consent
		for _, gvlID := range gvlIDs {
			result[gvlID] = false
		}
		return result
	}

	// Parse TCF consent string
	tcfData, err := m.parseTCFv2String(consentString)
	if err != nil {
		logger.Log.Debug().
			Err(err).
			Msg("Failed to parse TCF string for vendor consent check")
		for _, gvlID := range gvlIDs {
			result[gvlID] = false
		}
		return result
	}

	if tcfData == nil || tcfData.VendorConsents == nil {
		for _, gvlID := range gvlIDs {
			result[gvlID] = false
		}
		return result
	}

	// Check each vendor
	for _, gvlID := range gvlIDs {
		hasConsent, exists := tcfData.VendorConsents[gvlID]
		result[gvlID] = exists && hasConsent
	}

	return result
}

// CheckVendorConsentStatic is a standalone function that can be called without a middleware instance
// This is useful for the exchange to check vendor consents during auction
func CheckVendorConsentStatic(consentString string, gvlID int) bool {
	if consentString == "" || gvlID <= 0 {
		return false
	}

	// Create a temporary middleware to use its parsing logic
	m := &PrivacyMiddleware{}
	tcfData, err := m.parseTCFv2String(consentString)
	if err != nil {
		return false
	}

	if tcfData == nil || tcfData.VendorConsents == nil {
		return false
	}

	hasConsent, exists := tcfData.VendorConsents[gvlID]
	return exists && hasConsent
}

// DetectRegulationFromGeo determines which privacy regulation applies based on device geo
// This is a standalone function for use in the exchange during auction
func DetectRegulationFromGeo(geo *openrtb.Geo) PrivacyRegulation {
	if geo == nil {
		return RegulationNone
	}

	// Check GDPR countries (EU/EEA + UK)
	if geo.Country != "" && gdprCountries[geo.Country] {
		return RegulationGDPR
	}

	// Check US state privacy laws
	if geo.Country == "USA" && geo.Region != "" {
		if regulation, exists := usPrivacyStates[geo.Region]; exists {
			return regulation
		}
		return RegulationNone
	}

	// Check other countries with privacy laws
	switch geo.Country {
	case "BRA":
		return RegulationLGPD
	case "CAN":
		return RegulationPIPEDA
	case "SGP":
		return RegulationPDPA
	}

	return RegulationNone
}

// ShouldFilterBidderByGeo checks if a bidder should be filtered based on geo and consent
// Returns true if bidder should be SKIPPED (filtered out)
func ShouldFilterBidderByGeo(req *openrtb.BidRequest, gvlID int) bool {
	if req == nil || req.Device == nil || req.Device.Geo == nil {
		return false // No geo data, can't filter
	}

	regulation := DetectRegulationFromGeo(req.Device.Geo)

	switch regulation {
	case RegulationGDPR:
		// For GDPR, check if regs.gdpr is set and if bidder has consent
		if req.Regs != nil && req.Regs.GDPR != nil && *req.Regs.GDPR == 1 {
			// GDPR applies - check vendor consent
			if gvlID > 0 {
				consentString := ""
				if req.User != nil {
					consentString = req.User.Consent
				}
				// Filter out (return true) if no consent
				return !CheckVendorConsentStatic(consentString, gvlID)
			}
		}

	case RegulationCCPA, RegulationVCDPA, RegulationCPA, RegulationCTDPA, RegulationUCPA:
		// For US privacy states, check if user has opted out
		if req.Regs != nil && len(req.Regs.USPrivacy) >= 3 {
			// Position 2 in US Privacy String indicates opt-out
			// 'Y' means user HAS opted out (filter the bidder)
			// 'N' means user has NOT opted out (allow the bidder)
			optOut := req.Regs.USPrivacy[2]
			return optOut == 'Y' // Filter if opted out
		}

	case RegulationLGPD, RegulationPIPEDA, RegulationPDPA:
		// Other regulations not yet fully implemented
		// Don't filter for now
		return false

	case RegulationNone:
		// No applicable regulation
		return false
	}

	return false
}

// TCF parsing errors
var (
	errInvalidTCFLength   = &tcfError{"consent string too short"}
	errInvalidTCFEncoding = &tcfError{"invalid base64 encoding"}
	errInvalidTCFVersion  = &tcfError{"unsupported TCF version"}
)

type tcfError struct{ msg string }

func (e *tcfError) Error() string { return e.msg }

// bitReader reads bits from a byte slice
type bitReader struct {
	data   []byte
	bitPos int
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data, bitPos: 0}
}

func (r *bitReader) readBool() bool {
	if r.bitPos/8 >= len(r.data) {
		return false
	}
	bytePos := r.bitPos / 8
	bitOffset := 7 - (r.bitPos % 8)
	r.bitPos++
	return (r.data[bytePos] >> bitOffset & 1) == 1
}

func (r *bitReader) readInt(bits int) int {
	result := 0
	for i := 0; i < bits; i++ {
		result = result << 1
		if r.readBool() {
			result |= 1
		}
	}
	return result
}

// isValidTCFv2String performs basic validation of a TCF v2 consent string
// P0-4: This is a lightweight check - full parsing happens in IDR
func (m *PrivacyMiddleware) isValidTCFv2String(consent string) bool {
	if consent == "" {
		return false
	}

	// TCF v2 strings should be base64url encoded
	// Minimum reasonable length for a TCF v2 string is ~20 chars
	if len(consent) < 20 {
		return false
	}

	// Try to decode the string
	decoded, err := base64.RawURLEncoding.DecodeString(consent)
	if err != nil {
		// Try with standard base64 (some implementations use this)
		decoded, err = base64.StdEncoding.DecodeString(consent)
		if err != nil {
			return false
		}
	}

	// Minimum decoded length for TCF v2
	if len(decoded) < 5 {
		return false
	}

	// Extract version from first 6 bits
	version := (decoded[0] >> 2) & 0x3F

	// TCF v2 should have version 2
	// We also accept version 1 for backwards compat, but log a warning
	if version != 2 && version != 1 {
		logger.Log.Debug().
			Int("version", int(version)).
			Msg("Unexpected TCF version in consent string")
		return false
	}

	if version == 1 {
		logger.Log.Warn().Msg("TCF v1 consent string detected - consider upgrading to v2")
	}

	return true
}

// checkCCPACompliance validates US Privacy (CCPA) signals and enforces opt-out
func (m *PrivacyMiddleware) checkCCPACompliance(requestID, usPrivacy string) *PrivacyViolation {
	// US Privacy string format: VNOS (Version, Notice, OptOut, LSPA)
	// Position 0: Version (1)
	// Position 1: Explicit Notice (Y/N/-)
	// Position 2: Opt-Out of Sale (Y/N/-)
	// Position 3: LSPA Covered (Y/N/-)
	//
	// Examples:
	// 1YNY = v1, notice given, NOT opted out, LSPA covered
	// 1YYN = v1, notice given, OPTED OUT of sale, not LSPA covered
	// 1--- = v1, all signals not applicable

	if len(usPrivacy) < 4 {
		logger.Log.Debug().
			Str("request_id", requestID).
			Str("us_privacy", usPrivacy).
			Msg("Invalid US Privacy string format (too short)")
		return nil // Don't block on malformed strings, just log
	}

	// Validate version
	version := usPrivacy[0]
	if version != '1' {
		logger.Log.Debug().
			Str("request_id", requestID).
			Str("us_privacy", usPrivacy).
			Msg("Unknown US Privacy version")
		return nil
	}

	// Check opt-out signal (position 2)
	optOut := usPrivacy[2]

	if optOut == 'Y' {
		logger.Log.Info().
			Str("request_id", requestID).
			Str("us_privacy", usPrivacy).
			Msg("CCPA opt-out signal received")

		// P0: Enforce CCPA opt-out if configured
		if m.config.EnforceCCPA {
			return &PrivacyViolation{
				Regulation:  "CCPA",
				Reason:      "User has opted out of data sale under CCPA",
				NoBidReason: openrtb.NoBidAdsNotAllowed,
			}
		}
	}

	// Log notice status for compliance auditing
	notice := usPrivacy[1]
	if notice == 'N' {
		logger.Log.Warn().
			Str("request_id", requestID).
			Str("us_privacy", usPrivacy).
			Msg("CCPA: Explicit notice not provided to user")
	}

	return nil
}

// P2-2: IP Anonymization for GDPR Compliance
// These functions implement privacy-preserving IP address masking as recommended
// by GDPR guidelines and the German DPA (Datenschutzkonferenz).

// AnonymizeIPv4 masks the last octet of an IPv4 address
// Example: "192.168.1.100" -> "192.168.1.0"
func AnonymizeIPv4(ip net.IP) string {
	if ip == nil {
		return ""
	}
	ipv4 := ip.To4()
	if ipv4 == nil {
		return ip.String()
	}
	// Zero out the last octet
	ipv4[3] = 0
	return ipv4.String()
}

// AnonymizeIPv6 masks the last 80 bits of an IPv6 address, keeping only the first 48 bits
// This follows the recommendation to mask at minimum /48 for IPv6
// Example: "2001:0db8:85a3:0000:0000:8a2e:0370:7334" -> "2001:db8:85a3::"
func AnonymizeIPv6(ip net.IP) string {
	if ip == nil {
		return ""
	}
	ipv6 := ip.To16()
	if ipv6 == nil {
		return ip.String()
	}
	// Keep first 48 bits (6 bytes), zero out the rest
	for i := 6; i < 16; i++ {
		ipv6[i] = 0
	}
	return ipv6.String()
}

// AnonymizeIP detects IP version and applies appropriate anonymization
// Returns the anonymized IP string, or empty string if input is invalid
func AnonymizeIP(ipStr string) string {
	if ipStr == "" {
		return ""
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "" // Invalid IP, remove entirely
	}
	// Check if it's IPv4 (can be represented as 4 bytes)
	if ip.To4() != nil {
		return AnonymizeIPv4(ip)
	}
	// Otherwise treat as IPv6
	return AnonymizeIPv6(ip)
}

// anonymizeRequestIPs modifies the bid request to anonymize IP addresses
// This is called when GDPR applies and IP anonymization is enabled
func (m *PrivacyMiddleware) anonymizeRequestIPs(req *openrtb.BidRequest) {
	if req.Device == nil {
		return
	}

	if req.Device.IP != "" {
		originalIP := req.Device.IP
		req.Device.IP = AnonymizeIP(originalIP)
		logger.Log.Debug().
			Str("request_id", req.ID).
			Str("original_ip", originalIP).
			Str("anonymized_ip", req.Device.IP).
			Msg("P2-2: Anonymized IPv4 for GDPR compliance")
	}

	if req.Device.IPv6 != "" {
		originalIPv6 := req.Device.IPv6
		req.Device.IPv6 = AnonymizeIP(originalIPv6)
		logger.Log.Debug().
			Str("request_id", req.ID).
			Str("original_ipv6", originalIPv6).
			Str("anonymized_ipv6", req.Device.IPv6).
			Msg("P2-2: Anonymized IPv6 for GDPR compliance")
	}
}
