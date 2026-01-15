package adapters

import (
	"errors"
	"strings"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestBidderError_Error_WithCause(t *testing.T) {
	cause := errors.New("underlying error")
	err := &BidderError{
		BidderCode: "appnexus",
		Code:       ErrorCodeMarshal,
		Message:    "failed to marshal",
		Cause:      cause,
	}

	result := err.Error()
	if !strings.Contains(result, "[MARSHAL_ERROR]") {
		t.Errorf("expected MARSHAL_ERROR code in: %s", result)
	}
	if !strings.Contains(result, "appnexus") {
		t.Errorf("expected bidder code in: %s", result)
	}
	if !strings.Contains(result, "failed to marshal") {
		t.Errorf("expected message in: %s", result)
	}
	if !strings.Contains(result, "underlying error") {
		t.Errorf("expected cause in: %s", result)
	}
}

func TestBidderError_Error_WithoutCause(t *testing.T) {
	err := &BidderError{
		BidderCode: "rubicon",
		Code:       ErrorCodeBadStatus,
		Message:    "unexpected status",
	}

	result := err.Error()
	if !strings.Contains(result, "[BAD_STATUS]") {
		t.Errorf("expected BAD_STATUS code in: %s", result)
	}
	if !strings.Contains(result, "rubicon") {
		t.Errorf("expected bidder code in: %s", result)
	}
	if strings.Contains(result, "(") {
		t.Errorf("should not have parentheses without cause: %s", result)
	}
}

func TestBidderError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &BidderError{
		BidderCode: "test",
		Code:       ErrorCodeParse,
		Message:    "parse failed",
		Cause:      cause,
	}

	unwrapped := err.Unwrap()
	if !errors.Is(unwrapped, cause) {
		t.Errorf("expected unwrapped to be cause")
	}
}

func TestBidderError_Unwrap_NilCause(t *testing.T) {
	err := &BidderError{
		BidderCode: "test",
		Code:       ErrorCodeParse,
		Message:    "parse failed",
	}

	unwrapped := err.Unwrap()
	if unwrapped != nil {
		t.Errorf("expected nil unwrap")
	}
}

func TestNewMarshalError(t *testing.T) {
	cause := errors.New("json error")
	err := NewMarshalError("appnexus", cause)

	if err.BidderCode != "appnexus" {
		t.Errorf("expected appnexus, got %s", err.BidderCode)
	}
	if err.Code != ErrorCodeMarshal {
		t.Errorf("expected MARSHAL_ERROR, got %s", err.Code)
	}
	if !errors.Is(err.Cause, cause) {
		t.Error("expected cause to be set")
	}
	if !strings.Contains(err.Message, "marshal") {
		t.Errorf("expected 'marshal' in message: %s", err.Message)
	}
}

func TestNewBadRequestError(t *testing.T) {
	err := NewBadRequestError("rubicon", "invalid impression")

	if err.BidderCode != "rubicon" {
		t.Errorf("expected rubicon, got %s", err.BidderCode)
	}
	if err.Code != ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %s", err.Code)
	}
	if err.Cause != nil {
		t.Error("expected no cause")
	}
	if !strings.Contains(err.Message, "invalid impression") {
		t.Errorf("expected response body in message: %s", err.Message)
	}
}

func TestNewBadStatusError(t *testing.T) {
	err := NewBadStatusError("pubmatic", 503)

	if err.BidderCode != "pubmatic" {
		t.Errorf("expected pubmatic, got %s", err.BidderCode)
	}
	if err.Code != ErrorCodeBadStatus {
		t.Errorf("expected BAD_STATUS, got %s", err.Code)
	}
	if !strings.Contains(err.Message, "503") {
		t.Errorf("expected status code in message: %s", err.Message)
	}
}

func TestNewParseError(t *testing.T) {
	cause := errors.New("json: unexpected token")
	err := NewParseError("criteo", cause)

	if err.BidderCode != "criteo" {
		t.Errorf("expected criteo, got %s", err.BidderCode)
	}
	if err.Code != ErrorCodeParse {
		t.Errorf("expected PARSE_ERROR, got %s", err.Code)
	}
	if !errors.Is(err.Cause, cause) {
		t.Error("expected cause to be set")
	}
}

func TestErrorCodeConstants(t *testing.T) {
	tests := []struct {
		code     BidderErrorCode
		expected string
	}{
		{ErrorCodeMarshal, "MARSHAL_ERROR"},
		{ErrorCodeBadRequest, "BAD_REQUEST"},
		{ErrorCodeBadStatus, "BAD_STATUS"},
		{ErrorCodeParse, "PARSE_ERROR"},
		{ErrorCodeTimeout, "TIMEOUT"},
		{ErrorCodeConnection, "CONNECTION_ERROR"},
	}

	for _, tt := range tests {
		if string(tt.code) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.code)
		}
	}
}

func TestBuildImpMap(t *testing.T) {
	imps := []openrtb.Imp{
		{ID: "imp-1", Banner: &openrtb.Banner{}},
		{ID: "imp-2", Video: &openrtb.Video{}},
		{ID: "imp-3", Native: &openrtb.Native{}},
	}

	impMap := BuildImpMap(imps)

	if len(impMap) != 3 {
		t.Errorf("expected 3 entries, got %d", len(impMap))
	}

	if impMap["imp-1"] == nil {
		t.Error("expected imp-1 in map")
	}
	if impMap["imp-1"].Banner == nil {
		t.Error("expected banner for imp-1")
	}

	if impMap["imp-2"] == nil {
		t.Error("expected imp-2 in map")
	}
	if impMap["imp-2"].Video == nil {
		t.Error("expected video for imp-2")
	}

	if impMap["imp-3"] == nil {
		t.Error("expected imp-3 in map")
	}
}

func TestBuildImpMap_Empty(t *testing.T) {
	impMap := BuildImpMap([]openrtb.Imp{})
	if len(impMap) != 0 {
		t.Errorf("expected empty map, got %d entries", len(impMap))
	}
}

func TestBuildImpMap_DuplicateIDs(t *testing.T) {
	// Last one should win
	imps := []openrtb.Imp{
		{ID: "imp-1", Banner: &openrtb.Banner{W: 300}},
		{ID: "imp-1", Banner: &openrtb.Banner{W: 728}},
	}

	impMap := BuildImpMap(imps)
	if len(impMap) != 1 {
		t.Errorf("expected 1 entry, got %d", len(impMap))
	}
	if impMap["imp-1"].Banner.W != 728 {
		t.Error("expected last impression to win")
	}
}

func TestGetBidTypeFromMap_Banner(t *testing.T) {
	impMap := map[string]*openrtb.Imp{
		"imp-1": {ID: "imp-1", Banner: &openrtb.Banner{}},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidTypeFromMap(bid, impMap)
	if bidType != BidTypeBanner {
		t.Errorf("expected banner, got %s", bidType)
	}
}

func TestGetBidTypeFromMap_Video(t *testing.T) {
	impMap := map[string]*openrtb.Imp{
		"imp-1": {ID: "imp-1", Video: &openrtb.Video{}},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidTypeFromMap(bid, impMap)
	if bidType != BidTypeVideo {
		t.Errorf("expected video, got %s", bidType)
	}
}

func TestGetBidTypeFromMap_Native(t *testing.T) {
	impMap := map[string]*openrtb.Imp{
		"imp-1": {ID: "imp-1", Native: &openrtb.Native{}},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidTypeFromMap(bid, impMap)
	if bidType != BidTypeNative {
		t.Errorf("expected native, got %s", bidType)
	}
}

func TestGetBidTypeFromMap_Audio(t *testing.T) {
	impMap := map[string]*openrtb.Imp{
		"imp-1": {ID: "imp-1", Audio: &openrtb.Audio{}},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidTypeFromMap(bid, impMap)
	if bidType != BidTypeAudio {
		t.Errorf("expected audio, got %s", bidType)
	}
}

func TestGetBidTypeFromMap_NotFound(t *testing.T) {
	impMap := map[string]*openrtb.Imp{
		"imp-1": {ID: "imp-1", Banner: &openrtb.Banner{}},
	}
	bid := &openrtb.Bid{ImpID: "imp-unknown"}

	bidType := GetBidTypeFromMap(bid, impMap)
	if bidType != BidTypeBanner {
		t.Errorf("expected banner (default), got %s", bidType)
	}
}

func TestGetBidTypeFromMap_VideoPriority(t *testing.T) {
	// Video should take priority if both video and banner exist
	impMap := map[string]*openrtb.Imp{
		"imp-1": {
			ID:     "imp-1",
			Video:  &openrtb.Video{},
			Banner: &openrtb.Banner{},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidTypeFromMap(bid, impMap)
	if bidType != BidTypeVideo {
		t.Errorf("expected video (priority), got %s", bidType)
	}
}

func TestGetBidType_Banner(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{}},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidType(bid, request)
	if bidType != BidTypeBanner {
		t.Errorf("expected banner, got %s", bidType)
	}
}

func TestGetBidType_Video(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Video: &openrtb.Video{}},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidType(bid, request)
	if bidType != BidTypeVideo {
		t.Errorf("expected video, got %s", bidType)
	}
}

func TestGetBidType_Native(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Native: &openrtb.Native{}},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidType(bid, request)
	if bidType != BidTypeNative {
		t.Errorf("expected native, got %s", bidType)
	}
}

func TestGetBidType_Audio(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Audio: &openrtb.Audio{}},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidType(bid, request)
	if bidType != BidTypeAudio {
		t.Errorf("expected audio, got %s", bidType)
	}
}

func TestGetBidType_NotFound(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{}},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-unknown"}

	bidType := GetBidType(bid, request)
	if bidType != BidTypeBanner {
		t.Errorf("expected banner (default), got %s", bidType)
	}
}

func TestGetBidType_MultipleImpressions(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{}},
			{ID: "imp-2", Video: &openrtb.Video{}},
			{ID: "imp-3", Native: &openrtb.Native{}},
		},
	}

	tests := []struct {
		impID    string
		expected BidType
	}{
		{"imp-1", BidTypeBanner},
		{"imp-2", BidTypeVideo},
		{"imp-3", BidTypeNative},
	}

	for _, tt := range tests {
		bid := &openrtb.Bid{ImpID: tt.impID}
		bidType := GetBidType(bid, request)
		if bidType != tt.expected {
			t.Errorf("for %s: expected %s, got %s", tt.impID, tt.expected, bidType)
		}
	}
}

func TestGetBidType_EmptyRequest(t *testing.T) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	bidType := GetBidType(bid, request)
	if bidType != BidTypeBanner {
		t.Errorf("expected banner (default), got %s", bidType)
	}
}

// Benchmark tests
func BenchmarkBuildImpMap(b *testing.B) {
	imps := make([]openrtb.Imp, 10)
	for i := 0; i < 10; i++ {
		imps[i] = openrtb.Imp{ID: string(rune('a' + i)), Banner: &openrtb.Banner{}}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildImpMap(imps)
	}
}

func BenchmarkGetBidTypeFromMap(b *testing.B) {
	impMap := map[string]*openrtb.Imp{
		"imp-1": {ID: "imp-1", Video: &openrtb.Video{}},
	}
	bid := &openrtb.Bid{ImpID: "imp-1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetBidTypeFromMap(bid, impMap)
	}
}

func BenchmarkGetBidType(b *testing.B) {
	request := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{}},
			{ID: "imp-2", Video: &openrtb.Video{}},
			{ID: "imp-3", Native: &openrtb.Native{}},
			{ID: "imp-4", Audio: &openrtb.Audio{}},
			{ID: "imp-5", Banner: &openrtb.Banner{}},
		},
	}
	bid := &openrtb.Bid{ImpID: "imp-5"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetBidType(bid, request)
	}
}
