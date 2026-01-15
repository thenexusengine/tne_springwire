// Package adapters provides the bidder adapter framework
package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// P3-3: Standard error codes for consistent error handling
type BidderErrorCode string

const (
	ErrorCodeMarshal    BidderErrorCode = "MARSHAL_ERROR"
	ErrorCodeBadRequest BidderErrorCode = "BAD_REQUEST"
	ErrorCodeBadStatus  BidderErrorCode = "BAD_STATUS"
	ErrorCodeParse      BidderErrorCode = "PARSE_ERROR"
	ErrorCodeTimeout    BidderErrorCode = "TIMEOUT"
	ErrorCodeConnection BidderErrorCode = "CONNECTION_ERROR"
)

// BidderError represents a standardized adapter error
type BidderError struct {
	BidderCode string
	Code       BidderErrorCode
	Message    string
	Cause      error
}

func (e *BidderError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %s (%v)", e.Code, e.BidderCode, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.BidderCode, e.Message)
}

func (e *BidderError) Unwrap() error {
	return e.Cause
}

// P3-3: Standard error constructors for consistent error formatting

// NewMarshalError creates a standardized marshal error
func NewMarshalError(bidderCode string, cause error) *BidderError {
	return &BidderError{
		BidderCode: bidderCode,
		Code:       ErrorCodeMarshal,
		Message:    "failed to marshal request",
		Cause:      cause,
	}
}

// NewBadRequestError creates a standardized bad request error
func NewBadRequestError(bidderCode string, responseBody string) *BidderError {
	return &BidderError{
		BidderCode: bidderCode,
		Code:       ErrorCodeBadRequest,
		Message:    fmt.Sprintf("bad request: %s", responseBody),
	}
}

// NewBadStatusError creates a standardized status code error
func NewBadStatusError(bidderCode string, statusCode int) *BidderError {
	return &BidderError{
		BidderCode: bidderCode,
		Code:       ErrorCodeBadStatus,
		Message:    fmt.Sprintf("unexpected status: %d", statusCode),
	}
}

// NewParseError creates a standardized parse error
func NewParseError(bidderCode string, cause error) *BidderError {
	return &BidderError{
		BidderCode: bidderCode,
		Code:       ErrorCodeParse,
		Message:    "failed to parse response",
		Cause:      cause,
	}
}

// P2-3: BuildImpMap creates a map of impression ID to impression for O(1) lookups
// Use this instead of iterating through impressions for each bid
func BuildImpMap(imps []openrtb.Imp) map[string]*openrtb.Imp {
	impMap := make(map[string]*openrtb.Imp, len(imps))
	for i := range imps {
		impMap[imps[i].ID] = &imps[i]
	}
	return impMap
}

// P2-3: GetBidTypeFromMap determines bid type using pre-built impression map (O(1))
func GetBidTypeFromMap(bid *openrtb.Bid, impMap map[string]*openrtb.Imp) BidType {
	imp, ok := impMap[bid.ImpID]
	if !ok {
		return BidTypeBanner
	}

	if imp.Video != nil {
		return BidTypeVideo
	}
	if imp.Native != nil {
		return BidTypeNative
	}
	if imp.Audio != nil {
		return BidTypeAudio
	}
	return BidTypeBanner
}

// P2-3: GetBidType determines bid type from impression (convenience wrapper)
// Note: For multiple bids, use BuildImpMap + GetBidTypeFromMap for better performance
func GetBidType(bid *openrtb.Bid, request *openrtb.BidRequest) BidType {
	for _, imp := range request.Imp {
		if imp.ID == bid.ImpID {
			if imp.Video != nil {
				return BidTypeVideo
			}
			if imp.Native != nil {
				return BidTypeNative
			}
			if imp.Audio != nil {
				return BidTypeAudio
			}
			return BidTypeBanner
		}
	}
	return BidTypeBanner
}

// P2-5: SimpleAdapter provides common OpenRTB adapter functionality
// Simple bidders can embed this to reduce boilerplate code.
// This handles the common pattern of: POST JSON -> Parse JSON response -> Extract bids
type SimpleAdapter struct {
	BidderCode     string  // Bidder code for error messages
	Endpoint       string  // Bidder endpoint URL
	DefaultBidType BidType // Default bid type if can't be determined from impression
}

// NewSimpleAdapter creates a new SimpleAdapter with the given configuration
func NewSimpleAdapter(bidderCode, endpoint string, defaultBidType BidType) *SimpleAdapter {
	return &SimpleAdapter{
		BidderCode:     bidderCode,
		Endpoint:       endpoint,
		DefaultBidType: defaultBidType,
	}
}

// MakeRequests implements the standard ORTB JSON POST pattern
func (a *SimpleAdapter) MakeRequests(request *openrtb.BidRequest, extraInfo *ExtraRequestInfo) ([]*RequestData, []error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, []error{NewMarshalError(a.BidderCode, err)}
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	return []*RequestData{{
		Method:  "POST",
		URI:     a.Endpoint,
		Body:    body,
		Headers: headers,
	}}, nil
}

// MakeBids implements the standard ORTB response parsing pattern
func (a *SimpleAdapter) MakeBids(request *openrtb.BidRequest, responseData *ResponseData) (*BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil // No bids
	}
	if responseData.StatusCode != http.StatusOK {
		return nil, []error{NewBadStatusError(a.BidderCode, responseData.StatusCode)}
	}

	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{NewParseError(a.BidderCode, err)}
	}

	// Build impression map for O(1) bid type lookup
	impMap := BuildImpMap(request.Imp)

	response := &BidderResponse{
		Currency:   bidResp.Cur,
		ResponseID: bidResp.ID,
		Bids:       make([]*TypedBid, 0, len(bidResp.SeatBid)),
	}

	for _, seatBid := range bidResp.SeatBid {
		for i := range seatBid.Bid {
			bid := &seatBid.Bid[i]
			bidType := a.DefaultBidType
			if bidType == "" {
				bidType = GetBidTypeFromMap(bid, impMap)
			}
			response.Bids = append(response.Bids, &TypedBid{
				Bid:     bid,
				BidType: bidType,
			})
		}
	}

	return response, nil
}

// MakeBidsWithType is a convenience wrapper that always uses the default bid type
func (a *SimpleAdapter) MakeBidsWithType(request *openrtb.BidRequest, responseData *ResponseData, bidType BidType) (*BidderResponse, []error) {
	orig := a.DefaultBidType
	a.DefaultBidType = bidType
	result, errs := a.MakeBids(request, responseData)
	a.DefaultBidType = orig
	return result, errs
}
