package fpd

import (
	"encoding/json"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestNewProcessor(t *testing.T) {
	p := NewProcessor(nil)
	if p == nil {
		t.Fatal("expected non-nil processor")
	}

	config := p.GetConfig()
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	// Should have default config values
	if !config.Enabled {
		t.Error("expected FPD to be enabled by default")
	}
}

func TestProcessorDisabled(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled: false,
	})

	req := &openrtb.BidRequest{
		ID: "test-req",
	}

	result, err := p.ProcessRequest(req, []string{"bidder1", "bidder2"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when FPD disabled")
	}
}

func TestProcessorExtractsSiteData(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled:     true,
		SiteEnabled: true,
	})

	siteExt := json.RawMessage(`{"data":{"segment":"premium"}}`)
	req := &openrtb.BidRequest{
		ID: "test-req",
		Site: &openrtb.Site{
			ID:   "site1",
			Name: "Test Site",
			Ext:  siteExt,
		},
	}

	result, err := p.ProcessRequest(req, []string{"bidder1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	bidderFPD := result["bidder1"]
	if bidderFPD == nil {
		t.Fatal("expected FPD for bidder1")
	}
	if bidderFPD.Site == nil {
		t.Error("expected site FPD data")
	}
}

func TestProcessorExtractsUserData(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled:     true,
		UserEnabled: true,
	})

	userExt := json.RawMessage(`{"data":{"audience":"sports"}}`)
	req := &openrtb.BidRequest{
		ID: "test-req",
		User: &openrtb.User{
			ID:  "user1",
			Ext: userExt,
		},
	}

	result, err := p.ProcessRequest(req, []string{"bidder1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bidderFPD := result["bidder1"]
	if bidderFPD == nil {
		t.Fatal("expected FPD for bidder1")
	}
	if bidderFPD.User == nil {
		t.Error("expected user FPD data")
	}
}

func TestProcessorExtractsImpData(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled:    true,
		ImpEnabled: true,
	})

	impExt := json.RawMessage(`{"data":{"position":"atf"}}`)
	req := &openrtb.BidRequest{
		ID: "test-req",
		Imp: []openrtb.Imp{
			{
				ID:  "imp1",
				Ext: impExt,
			},
		},
	}

	result, err := p.ProcessRequest(req, []string{"bidder1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bidderFPD := result["bidder1"]
	if bidderFPD == nil {
		t.Fatal("expected FPD for bidder1")
	}
	if len(bidderFPD.Imp) == 0 {
		t.Error("expected impression FPD data")
	}
	if _, ok := bidderFPD.Imp["imp1"]; !ok {
		t.Error("expected FPD data for imp1")
	}
}

func TestProcessorMultipleBidders(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled:     true,
		SiteEnabled: true,
	})

	siteExt := json.RawMessage(`{"data":{"test":"value"}}`)
	req := &openrtb.BidRequest{
		ID: "test-req",
		Site: &openrtb.Site{
			Ext: siteExt,
		},
	}

	bidders := []string{"bidder1", "bidder2", "bidder3"}
	result, err := p.ProcessRequest(req, bidders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, bidder := range bidders {
		if result[bidder] == nil {
			t.Errorf("expected FPD for %s", bidder)
		}
	}
}

func TestApplyFPDToRequest(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled:     true,
		SiteEnabled: true,
	})

	fpd := &ResolvedFPD{
		Site: json.RawMessage(`{"segment":"premium"}`),
	}

	req := &openrtb.BidRequest{
		ID:   "test-req",
		Site: &openrtb.Site{ID: "site1"},
	}

	err := p.ApplyFPDToRequest(req, "bidder1", fpd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Site.Ext == nil {
		t.Fatal("expected site ext to be set")
	}

	// Verify the data was applied
	var extObj map[string]json.RawMessage
	if err := json.Unmarshal(req.Site.Ext, &extObj); err != nil {
		t.Fatalf("failed to unmarshal ext: %v", err)
	}
	if _, ok := extObj["data"]; !ok {
		t.Error("expected data key in site ext")
	}
}

func TestProcessorGlobalFPD(t *testing.T) {
	p := NewProcessor(&Config{
		Enabled:       true,
		GlobalEnabled: true,
		SiteEnabled:   true,
	})

	// Request with global FPD in ext.prebid.data
	reqExt := json.RawMessage(`{
		"prebid": {
			"data": {
				"site": {"segment": "global_segment"}
			}
		}
	}`)

	req := &openrtb.BidRequest{
		ID:  "test-req",
		Ext: reqExt,
		Site: &openrtb.Site{
			ID: "site1",
		},
	}

	result, err := p.ProcessRequest(req, []string{"bidder1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	bidderFPD := result["bidder1"]
	if bidderFPD == nil {
		t.Fatal("expected FPD for bidder1")
	}
}

func TestMergeJSON(t *testing.T) {
	p := NewProcessor(nil)

	tests := []struct {
		name     string
		base     json.RawMessage
		overlay  json.RawMessage
		expected string
	}{
		{
			name:     "nil base",
			base:     nil,
			overlay:  json.RawMessage(`{"key":"value"}`),
			expected: `{"key":"value"}`,
		},
		{
			name:     "nil overlay",
			base:     json.RawMessage(`{"key":"value"}`),
			overlay:  nil,
			expected: `{"key":"value"}`,
		},
		{
			name:     "both nil",
			base:     nil,
			overlay:  nil,
			expected: "",
		},
		{
			name:     "merge objects",
			base:     json.RawMessage(`{"a":"1"}`),
			overlay:  json.RawMessage(`{"b":"2"}`),
			expected: `{"a":"1","b":"2"}`,
		},
		{
			name:     "overlay overwrites",
			base:     json.RawMessage(`{"a":"1"}`),
			overlay:  json.RawMessage(`{"a":"2"}`),
			expected: `{"a":"2"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.mergeJSON(tt.base, tt.overlay)

			if tt.expected == "" {
				if result != nil {
					t.Errorf("expected nil, got %s", string(result))
				}
				return
			}

			if result == nil {
				t.Fatalf("expected non-nil result")
			}

			// Parse both to compare
			var resultMap, expectedMap map[string]interface{}
			json.Unmarshal(result, &resultMap)
			json.Unmarshal([]byte(tt.expected), &expectedMap)

			if len(resultMap) != len(expectedMap) {
				t.Errorf("expected %v, got %v", expectedMap, resultMap)
			}
		})
	}
}

func TestCloneFPD(t *testing.T) {
	p := NewProcessor(nil)

	original := &ResolvedFPD{
		Site: json.RawMessage(`{"key":"value"}`),
		Imp:  map[string]json.RawMessage{"imp1": json.RawMessage(`{"data":"test"}`)},
	}

	clone := p.cloneFPD(original)

	// Modify original
	original.Site = json.RawMessage(`{"key":"modified"}`)
	original.Imp["imp1"] = json.RawMessage(`{"data":"modified"}`)

	// Clone should be unchanged
	if string(clone.Site) != `{"key":"value"}` {
		t.Errorf("clone site was modified: %s", string(clone.Site))
	}
}

func TestUpdateConfig(t *testing.T) {
	p := NewProcessor(&Config{Enabled: true})

	newConfig := &Config{
		Enabled:       false,
		SiteEnabled:   true,
		GlobalEnabled: true,
	}

	p.UpdateConfig(newConfig)

	config := p.GetConfig()
	if config.Enabled != false {
		t.Error("expected enabled to be false")
	}
	if config.GlobalEnabled != true {
		t.Error("expected global enabled to be true")
	}
}
