// Package openrtb provides OpenRTB 2.5/2.6 data models
package openrtb

import "encoding/json"

// BidRequest represents an OpenRTB 2.5/2.6 bid request
type BidRequest struct {
	ID     string          `json:"id"`
	Imp    []Imp           `json:"imp"`
	Site   *Site           `json:"site,omitempty"`
	App    *App            `json:"app,omitempty"`
	Device *Device         `json:"device,omitempty"`
	User   *User           `json:"user,omitempty"`
	Test   int             `json:"test,omitempty"`
	AT     int             `json:"at,omitempty"`    // Auction type: 1=first price, 2=second price
	TMax   int             `json:"tmax,omitempty"`  // Max time in ms for bid response
	WSeat  []string        `json:"wseat,omitempty"` // Allowed buyer seats
	BSeat  []string        `json:"bseat,omitempty"` // Blocked buyer seats
	AllImp int             `json:"allimps,omitempty"`
	Cur    []string        `json:"cur,omitempty"`   // Allowed currencies
	WLang  []string        `json:"wlang,omitempty"` // Allowed languages
	BCat   []string        `json:"bcat,omitempty"`  // Blocked categories
	BAdv   []string        `json:"badv,omitempty"`  // Blocked advertisers
	BApp   []string        `json:"bapp,omitempty"`  // Blocked apps
	Source *Source         `json:"source,omitempty"`
	Regs   *Regs           `json:"regs,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// Imp represents an impression object
type Imp struct {
	ID                string          `json:"id"`
	Metric            []Metric        `json:"metric,omitempty"`
	Banner            *Banner         `json:"banner,omitempty"`
	Video             *Video          `json:"video,omitempty"`
	Audio             *Audio          `json:"audio,omitempty"`
	Native            *Native         `json:"native,omitempty"`
	PMP               *PMP            `json:"pmp,omitempty"`
	DisplayManager    string          `json:"displaymanager,omitempty"`
	DisplayManagerVer string          `json:"displaymanagerver,omitempty"`
	Instl             int             `json:"instl,omitempty"` // Interstitial flag
	TagID             string          `json:"tagid,omitempty"`
	BidFloor          float64         `json:"bidfloor,omitempty"`
	BidFloorCur       string          `json:"bidfloorcur,omitempty"`
	ClickBrowser      int             `json:"clickbrowser,omitempty"`
	Secure            *int            `json:"secure,omitempty"`
	IframeBuster      []string        `json:"iframebuster,omitempty"`
	Exp               int             `json:"exp,omitempty"`
	Ext               json.RawMessage `json:"ext,omitempty"`
}

// Banner represents a banner impression
type Banner struct {
	Format   []Format        `json:"format,omitempty"`
	W        int             `json:"w,omitempty"`
	H        int             `json:"h,omitempty"`
	WMax     int             `json:"wmax,omitempty"`
	HMax     int             `json:"hmax,omitempty"`
	WMin     int             `json:"wmin,omitempty"`
	HMin     int             `json:"hmin,omitempty"`
	BType    []int           `json:"btype,omitempty"` // Blocked banner types
	BAttr    []int           `json:"battr,omitempty"` // Blocked creative attributes
	Pos      int             `json:"pos,omitempty"`   // Ad position
	Mimes    []string        `json:"mimes,omitempty"`
	TopFrame int             `json:"topframe,omitempty"`
	ExpDir   []int           `json:"expdir,omitempty"` // Expandable directions
	API      []int           `json:"api,omitempty"`
	ID       string          `json:"id,omitempty"`
	VCM      int             `json:"vcm,omitempty"`
	Ext      json.RawMessage `json:"ext,omitempty"`
}

// Format represents size format
type Format struct {
	W      int             `json:"w,omitempty"`
	H      int             `json:"h,omitempty"`
	WRatio int             `json:"wratio,omitempty"`
	HRatio int             `json:"hratio,omitempty"`
	WMin   int             `json:"wmin,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// Video represents a video impression
type Video struct {
	Mimes          []string        `json:"mimes,omitempty"`
	MinDuration    int             `json:"minduration,omitempty"`
	MaxDuration    int             `json:"maxduration,omitempty"`
	Protocols      []int           `json:"protocols,omitempty"`
	Protocol       int             `json:"protocol,omitempty"` // Deprecated
	W              int             `json:"w,omitempty"`
	H              int             `json:"h,omitempty"`
	StartDelay     *int            `json:"startdelay,omitempty"`
	Placement      int             `json:"placement,omitempty"`
	Linearity      int             `json:"linearity,omitempty"`
	Skip           *int            `json:"skip,omitempty"`
	SkipMin        int             `json:"skipmin,omitempty"`
	SkipAfter      int             `json:"skipafter,omitempty"`
	Sequence       int             `json:"sequence,omitempty"`
	BAttr          []int           `json:"battr,omitempty"`
	MaxExtended    int             `json:"maxextended,omitempty"`
	MinBitrate     int             `json:"minbitrate,omitempty"`
	MaxBitrate     int             `json:"maxbitrate,omitempty"`
	BoxingAllowed  int             `json:"boxingallowed,omitempty"`
	PlaybackMethod []int           `json:"playbackmethod,omitempty"`
	PlaybackEnd    int             `json:"playbackend,omitempty"`
	Delivery       []int           `json:"delivery,omitempty"`
	Pos            int             `json:"pos,omitempty"`
	CompanionAd    []Banner        `json:"companionad,omitempty"`
	API            []int           `json:"api,omitempty"`
	CompanionType  []int           `json:"companiontype,omitempty"`
	Ext            json.RawMessage `json:"ext,omitempty"`
}

// Audio represents an audio impression
type Audio struct {
	Mimes         []string        `json:"mimes,omitempty"`
	MinDuration   int             `json:"minduration,omitempty"`
	MaxDuration   int             `json:"maxduration,omitempty"`
	Protocols     []int           `json:"protocols,omitempty"`
	StartDelay    *int            `json:"startdelay,omitempty"`
	Sequence      int             `json:"sequence,omitempty"`
	BAttr         []int           `json:"battr,omitempty"`
	MaxExtended   int             `json:"maxextended,omitempty"`
	MinBitrate    int             `json:"minbitrate,omitempty"`
	MaxBitrate    int             `json:"maxbitrate,omitempty"`
	Delivery      []int           `json:"delivery,omitempty"`
	CompanionAd   []Banner        `json:"companionad,omitempty"`
	API           []int           `json:"api,omitempty"`
	CompanionType []int           `json:"companiontype,omitempty"`
	MaxSeq        int             `json:"maxseq,omitempty"`
	Feed          int             `json:"feed,omitempty"`
	Stitched      int             `json:"stitched,omitempty"`
	NVol          int             `json:"nvol,omitempty"`
	Ext           json.RawMessage `json:"ext,omitempty"`
}

// Native represents a native impression
type Native struct {
	Request string          `json:"request,omitempty"`
	Ver     string          `json:"ver,omitempty"`
	API     []int           `json:"api,omitempty"`
	BAttr   []int           `json:"battr,omitempty"`
	Ext     json.RawMessage `json:"ext,omitempty"`
}

// Metric represents a metric object
type Metric struct {
	Type   string          `json:"type,omitempty"`
	Value  float64         `json:"value,omitempty"`
	Vendor string          `json:"vendor,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// PMP represents a private marketplace
type PMP struct {
	PrivateAuction int             `json:"private_auction,omitempty"`
	Deals          []Deal          `json:"deals,omitempty"`
	Ext            json.RawMessage `json:"ext,omitempty"`
}

// Deal represents a deal object
type Deal struct {
	ID          string          `json:"id"`
	BidFloor    float64         `json:"bidfloor,omitempty"`
	BidFloorCur string          `json:"bidfloorcur,omitempty"`
	AT          int             `json:"at,omitempty"`
	WSeat       []string        `json:"wseat,omitempty"`
	WADomain    []string        `json:"wadomain,omitempty"`
	Ext         json.RawMessage `json:"ext,omitempty"`
}

// Site represents a website
type Site struct {
	ID            string          `json:"id,omitempty"`
	Name          string          `json:"name,omitempty"`
	Domain        string          `json:"domain,omitempty"`
	Cat           []string        `json:"cat,omitempty"`
	SectionCat    []string        `json:"sectioncat,omitempty"`
	PageCat       []string        `json:"pagecat,omitempty"`
	Page          string          `json:"page,omitempty"`
	Ref           string          `json:"ref,omitempty"`
	Search        string          `json:"search,omitempty"`
	Mobile        int             `json:"mobile,omitempty"`
	PrivacyPolicy int             `json:"privacypolicy,omitempty"`
	Publisher     *Publisher      `json:"publisher,omitempty"`
	Content       *Content        `json:"content,omitempty"`
	Keywords      string          `json:"keywords,omitempty"`
	Ext           json.RawMessage `json:"ext,omitempty"`
}

// App represents a mobile application
type App struct {
	ID            string          `json:"id,omitempty"`
	Name          string          `json:"name,omitempty"`
	Bundle        string          `json:"bundle,omitempty"`
	Domain        string          `json:"domain,omitempty"`
	StoreURL      string          `json:"storeurl,omitempty"`
	Cat           []string        `json:"cat,omitempty"`
	SectionCat    []string        `json:"sectioncat,omitempty"`
	PageCat       []string        `json:"pagecat,omitempty"`
	Ver           string          `json:"ver,omitempty"`
	PrivacyPolicy int             `json:"privacypolicy,omitempty"`
	Paid          int             `json:"paid,omitempty"`
	Publisher     *Publisher      `json:"publisher,omitempty"`
	Content       *Content        `json:"content,omitempty"`
	Keywords      string          `json:"keywords,omitempty"`
	Ext           json.RawMessage `json:"ext,omitempty"`
}

// Publisher represents a publisher
type Publisher struct {
	ID     string          `json:"id,omitempty"`
	Name   string          `json:"name,omitempty"`
	Cat    []string        `json:"cat,omitempty"`
	Domain string          `json:"domain,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// Content represents content information
type Content struct {
	ID                 string          `json:"id,omitempty"`
	Episode            int             `json:"episode,omitempty"`
	Title              string          `json:"title,omitempty"`
	Series             string          `json:"series,omitempty"`
	Season             string          `json:"season,omitempty"`
	Artist             string          `json:"artist,omitempty"`
	Genre              string          `json:"genre,omitempty"`
	Album              string          `json:"album,omitempty"`
	ISRC               string          `json:"isrc,omitempty"`
	Producer           *Producer       `json:"producer,omitempty"`
	URL                string          `json:"url,omitempty"`
	Cat                []string        `json:"cat,omitempty"`
	ProdQ              int             `json:"prodq,omitempty"`
	VideoQuality       int             `json:"videoquality,omitempty"` // Deprecated
	Context            int             `json:"context,omitempty"`
	ContentRating      string          `json:"contentrating,omitempty"`
	UserRating         string          `json:"userrating,omitempty"`
	QAGMediaRating     int             `json:"qagmediarating,omitempty"`
	Keywords           string          `json:"keywords,omitempty"`
	LiveStream         int             `json:"livestream,omitempty"`
	SourceRelationship int             `json:"sourcerelationship,omitempty"`
	Len                int             `json:"len,omitempty"`
	Language           string          `json:"language,omitempty"`
	Embeddable         int             `json:"embeddable,omitempty"`
	Data               []Data          `json:"data,omitempty"`
	Ext                json.RawMessage `json:"ext,omitempty"`
}

// Producer represents a content producer
type Producer struct {
	ID     string          `json:"id,omitempty"`
	Name   string          `json:"name,omitempty"`
	Cat    []string        `json:"cat,omitempty"`
	Domain string          `json:"domain,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// Device represents a user device
type Device struct {
	UA             string          `json:"ua,omitempty"`
	Geo            *Geo            `json:"geo,omitempty"`
	DNT            *int            `json:"dnt,omitempty"`
	Lmt            *int            `json:"lmt,omitempty"`
	IP             string          `json:"ip,omitempty"`
	IPv6           string          `json:"ipv6,omitempty"`
	DeviceType     int             `json:"devicetype,omitempty"`
	Make           string          `json:"make,omitempty"`
	Model          string          `json:"model,omitempty"`
	OS             string          `json:"os,omitempty"`
	OSV            string          `json:"osv,omitempty"`
	HWV            string          `json:"hwv,omitempty"`
	H              int             `json:"h,omitempty"`
	W              int             `json:"w,omitempty"`
	PPI            int             `json:"ppi,omitempty"`
	PxRatio        float64         `json:"pxratio,omitempty"`
	JS             int             `json:"js,omitempty"`
	GeoFetch       int             `json:"geofetch,omitempty"`
	FlashVer       string          `json:"flashver,omitempty"`
	Language       string          `json:"language,omitempty"`
	Carrier        string          `json:"carrier,omitempty"`
	MCCMNC         string          `json:"mccmnc,omitempty"`
	ConnectionType int             `json:"connectiontype,omitempty"`
	IFA            string          `json:"ifa,omitempty"`
	IDSHA1         string          `json:"didsha1,omitempty"`
	IDMD5          string          `json:"didmd5,omitempty"`
	DPIDSHA1       string          `json:"dpidsha1,omitempty"`
	DPIDMD5        string          `json:"dpidmd5,omitempty"`
	MacSHA1        string          `json:"macsha1,omitempty"`
	MacMD5         string          `json:"macmd5,omitempty"`
	Ext            json.RawMessage `json:"ext,omitempty"`
}

// Geo represents geographic location
type Geo struct {
	Lat           float64         `json:"lat,omitempty"`
	Lon           float64         `json:"lon,omitempty"`
	Type          int             `json:"type,omitempty"`
	Accuracy      int             `json:"accuracy,omitempty"`
	LastFix       int             `json:"lastfix,omitempty"`
	IPService     int             `json:"ipservice,omitempty"`
	Country       string          `json:"country,omitempty"`
	Region        string          `json:"region,omitempty"`
	RegionFIPS104 string          `json:"regionfips104,omitempty"`
	Metro         string          `json:"metro,omitempty"`
	City          string          `json:"city,omitempty"`
	ZIP           string          `json:"zip,omitempty"`
	UTCOffset     int             `json:"utcoffset,omitempty"`
	Ext           json.RawMessage `json:"ext,omitempty"`
}

// User represents a user
type User struct {
	ID         string          `json:"id,omitempty"`
	BuyerUID   string          `json:"buyeruid,omitempty"`
	YOB        int             `json:"yob,omitempty"`
	Gender     string          `json:"gender,omitempty"`
	Keywords   string          `json:"keywords,omitempty"`
	CustomData string          `json:"customdata,omitempty"`
	Geo        *Geo            `json:"geo,omitempty"`
	Data       []Data          `json:"data,omitempty"`
	Consent    string          `json:"consent,omitempty"`
	EIDs       []EID           `json:"eids,omitempty"`
	Ext        json.RawMessage `json:"ext,omitempty"`
}

// Data represents data segment
type Data struct {
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	Segment []Segment       `json:"segment,omitempty"`
	Ext     json.RawMessage `json:"ext,omitempty"`
}

// Segment represents a data segment
type Segment struct {
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Value string          `json:"value,omitempty"`
	Ext   json.RawMessage `json:"ext,omitempty"`
}

// EID represents extended identifier
type EID struct {
	Source string          `json:"source,omitempty"`
	UIDs   []UID           `json:"uids,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// UID represents a user ID
type UID struct {
	ID    string          `json:"id,omitempty"`
	AType int             `json:"atype,omitempty"`
	Ext   json.RawMessage `json:"ext,omitempty"`
}

// Source represents request source
type Source struct {
	FD     int             `json:"fd,omitempty"`
	TID    string          `json:"tid,omitempty"`
	PChain string          `json:"pchain,omitempty"`
	SChain *SupplyChain    `json:"schain,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// SupplyChain represents supply chain
type SupplyChain struct {
	Complete int               `json:"complete,omitempty"`
	Nodes    []SupplyChainNode `json:"nodes,omitempty"`
	Ver      string            `json:"ver,omitempty"`
	Ext      json.RawMessage   `json:"ext,omitempty"`
}

// SupplyChainNode represents a node in supply chain
type SupplyChainNode struct {
	ASI    string          `json:"asi,omitempty"`
	SID    string          `json:"sid,omitempty"`
	RID    string          `json:"rid,omitempty"`
	Name   string          `json:"name,omitempty"`
	Domain string          `json:"domain,omitempty"`
	HP     int             `json:"hp,omitempty"`
	Ext    json.RawMessage `json:"ext,omitempty"`
}

// Regs represents regulations
type Regs struct {
	COPPA     int             `json:"coppa,omitempty"`
	GDPR      *int            `json:"gdpr,omitempty"`
	USPrivacy string          `json:"us_privacy,omitempty"`
	GPP       string          `json:"gpp,omitempty"`
	GPPSID    []int           `json:"gpp_sid,omitempty"`
	Ext       json.RawMessage `json:"ext,omitempty"`
}
