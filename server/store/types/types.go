package types

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Uid is a database-specific record id, suitable to be used as a primary key.
type Uid uint64

// ZeroUid is a constant representing uninitialized Uid.
const ZeroUid Uid = 0

// Lengths of various Uid representations
const (
	uidBase64Unpadded = 11
	uidBase64Padded   = 12

	p2pBase64Unpadded = 22
	p2pBase64Padded   = 24
)

// IsZero checks if Uid is uninitialized.
func (uid Uid) IsZero() bool {
	return uid == 0
}

// Compare returns 0 if uid is equal to u2, 1 if u2 is greater than uid, -1 if u2 is smaller.
func (uid Uid) Compare(u2 Uid) int {
	if uid < u2 {
		return -1
	} else if uid > u2 {
		return 1
	}
	return 0
}

// MarshalBinary converts Uid to byte slice.
func (uid *Uid) MarshalBinary() ([]byte, error) {
	dst := make([]byte, 8)
	binary.LittleEndian.PutUint64(dst, uint64(*uid))
	return dst, nil
}

// UnmarshalBinary reads Uid from byte slice.
func (uid *Uid) UnmarshalBinary(b []byte) error {
	if len(b) < 8 {
		return errors.New("Uid.UnmarshalBinary: invalid length")
	}
	*uid = Uid(binary.LittleEndian.Uint64(b))
	return nil
}

// UnmarshalText reads Uid from string represented as byte slice.
func (uid *Uid) UnmarshalText(src []byte) error {
	if len(src) != uidBase64Unpadded {
		return errors.New("Uid.UnmarshalText: invalid length")
	}
	dec := make([]byte, base64.URLEncoding.DecodedLen(uidBase64Padded))
	for len(src) < uidBase64Padded {
		src = append(src, '=')
	}
	count, err := base64.URLEncoding.Decode(dec, src)
	if count < 8 {
		if err != nil {
			return errors.New("Uid.UnmarshalText: failed to decode " + err.Error())
		}
		return errors.New("Uid.UnmarshalText: failed to decode")
	}
	*uid = Uid(binary.LittleEndian.Uint64(dec))
	return nil
}

// MarshalText converts Uid to string represented as byte slice.
func (uid *Uid) MarshalText() ([]byte, error) {
	if *uid == 0 {
		return []byte{}, nil
	}
	src := make([]byte, 8)
	dst := make([]byte, base64.URLEncoding.EncodedLen(8))
	binary.LittleEndian.PutUint64(src, uint64(*uid))
	base64.URLEncoding.Encode(dst, src)
	return dst[0:uidBase64Unpadded], nil
}

// MarshalJSON converts Uid to double quoted ("ajjj") string.
func (uid *Uid) MarshalJSON() ([]byte, error) {
	dst, _ := uid.MarshalText()
	return append(append([]byte{'"'}, dst...), '"'), nil
}

// UnmarshalJSON reads Uid from a double quoted string.
func (uid *Uid) UnmarshalJSON(b []byte) error {
	size := len(b)
	if size != (uidBase64Unpadded + 2) {
		return errors.New("Uid.UnmarshalJSON: invalid length")
	} else if b[0] != '"' || b[size-1] != '"' {
		return errors.New("Uid.UnmarshalJSON: unrecognized")
	}
	return uid.UnmarshalText(b[1 : size-1])
}

// String converts Uid to string
func (uid Uid) String() string {
	buf, _ := uid.MarshalText()
	return string(buf)
}

/*
// Scan implements sql.Scanner interface
func (uid *Uid) Scan(i interface{}) error {
	return nil
}
*/

// ParseUid parses string NOT prefixed with anything
func ParseUid(s string) Uid {
	var uid Uid
	uid.UnmarshalText([]byte(s))
	return uid
}

// UserId converts Uid to string prefixed with 'usr', like usrXXXXX
func (uid Uid) UserId() string {
	return uid.PrefixId("usr")
}

// FndName generates 'fnd' topic name for the given Uid.
func (uid Uid) FndName() string {
	return uid.PrefixId("fnd")
}

// PrefixId converts Uid to string prefixed with the given prefix.
func (uid Uid) PrefixId(prefix string) string {
	if uid.IsZero() {
		return ""
	}
	return prefix + uid.String()
}

// ParseUserId parses user ID of the form "usrXXXXXX"
func ParseUserId(s string) Uid {
	var uid Uid
	if strings.HasPrefix(s, "usr") {
		(&uid).UnmarshalText([]byte(s)[3:])
	}
	return uid
}

// P2PName takes two Uids and generates a P2P topic name
func (uid Uid) P2PName(u2 Uid) string {
	if !uid.IsZero() && !u2.IsZero() {
		b1, _ := uid.MarshalBinary()
		b2, _ := u2.MarshalBinary()

		if uid < u2 {
			b1 = append(b1, b2...)
		} else if uid > u2 {
			b1 = append(b2, b1...)
		} else {
			// Explicitly disable P2P with self
			return ""
		}

		return "p2p" + base64.URLEncoding.EncodeToString(b1)[:p2pBase64Unpadded]
	}

	return ""
}

// ParseP2P extracts uids from the name of a p2p topic.
func ParseP2P(p2p string) (uid1, uid2 Uid, err error) {
	if strings.HasPrefix(p2p, "p2p") {
		src := []byte(p2p)[3:]
		if len(src) != p2pBase64Unpadded {
			err = errors.New("ParseP2P: invalid length")
			return
		}
		dec := make([]byte, base64.URLEncoding.DecodedLen(p2pBase64Padded))
		for len(src) < p2pBase64Padded {
			src = append(src, '=')
		}
		var count int
		count, err = base64.URLEncoding.Decode(dec, src)
		if count < 16 {
			if err != nil {
				err = errors.New("ParseP2P: failed to decode " + err.Error())
			}
			err = errors.New("ParseP2P: invalid decoded length")
			return
		}
		uid1 = Uid(binary.LittleEndian.Uint64(dec))
		uid2 = Uid(binary.LittleEndian.Uint64(dec[8:]))
	} else {
		err = errors.New("ParseP2P: missing or invalid prefix")
	}
	return
}

// ObjHeader is the header shared by all stored objects.
type ObjHeader struct {
	Id        string // using string to get around rethinkdb's problems with unit64
	id        Uid
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `json:"DeletedAt,omitempty"`
}

// Uid assigns Uid header field.
func (h *ObjHeader) Uid() Uid {
	if h.id.IsZero() && h.Id != "" {
		h.id.UnmarshalText([]byte(h.Id))
	}
	return h.id
}

// SetUid assigns given Uid to appropriate header fields.
func (h *ObjHeader) SetUid(uid Uid) {
	h.id = uid
	h.Id = uid.String()
}

// TimeNow returns current wall time in UTC rounded to milliseconds.
func TimeNow() time.Time {
	return time.Now().UTC().Round(time.Millisecond)
}

// InitTimes initializes time.Time variables in the header to current time.
func (h *ObjHeader) InitTimes() {
	if h.CreatedAt.IsZero() {
		h.CreatedAt = TimeNow()
	}
	h.UpdatedAt = h.CreatedAt
	h.DeletedAt = nil
}

// MergeTimes intelligently copies time.Time variables from h2 to h.
func (h *ObjHeader) MergeTimes(h2 *ObjHeader) {
	// Set the creation time to the earliest value
	if h.CreatedAt.IsZero() || (!h2.CreatedAt.IsZero() && h2.CreatedAt.Before(h.CreatedAt)) {
		h.CreatedAt = h2.CreatedAt
	}
	// Set the update time to the latest value
	if h.UpdatedAt.Before(h2.UpdatedAt) {
		h.UpdatedAt = h2.UpdatedAt
	}
	// Set deleted time to the latest value
	if h2.DeletedAt != nil && (h.DeletedAt == nil || h.DeletedAt.Before(*h2.DeletedAt)) {
		h.DeletedAt = h2.DeletedAt
	}
}

// IsDeleted returns true if the object is deleted.
func (h *ObjHeader) IsDeleted() bool {
	return h.DeletedAt != nil
}

// StringSlice is defined so Scanner and Valuer can be attached to it.
type StringSlice []string

// Scan implements sql.Scanner interface.
func (ss *StringSlice) Scan(val interface{}) error {
	return json.Unmarshal(val.([]byte), ss)
}

// Value implements sql/driver.Valuer interface.
func (ss StringSlice) Value() (driver.Value, error) {
	return json.Marshal(ss)
}

// GenericData is wrapper for Public/Private fields. MySQL JSON field requires a valid
// JSON object, but public/private could contain basic types, like a string. Must wrap it in an object.
type GenericData struct {
	R interface{}
}

// Scan implements sql.Scanner interface.
func (gd *GenericData) Scan(val interface{}) error {
	return json.Unmarshal(val.([]byte), gd)
}

// Value implements sql/driver.Valuer interface.
func (gd GenericData) Value() (driver.Value, error) {
	return json.Marshal(gd)
}

func (gd *GenericData) UnmarshalJSON(data []byte) error {
	if gd == nil {
		gd = &GenericData{}
	}
	// Unmarshalling into the inner object
	return json.Unmarshal(data, &gd.R)
}

func (gd *GenericData) MarshalJSON() ([]byte, error) {
	if gd == nil {
		return nil, nil
	}
	// Marshalling the inner object only
	return json.Marshal(gd.R)
}

// User is a representation of a DB-stored user record.
type User struct {
	ObjHeader
	// Currently unused: Unconfirmed, Active, etc.
	State int

	// Default access to user for P2P topics (used as default modeGiven)
	Access DefaultAccess

	// Values for 'me' topic:

	// Last time when the user joined 'me' topic, by User Agent
	LastSeen *time.Time
	// User agent provided when accessing the topic last time
	UserAgent string

	Public interface{}

	// Unique indexed tags (email, phone) for finding this user. Stored on the
	// 'users' as well as indexed in 'tagunique'
	Tags StringSlice

	// Info on known devices, used for push notifications
	Devices map[string]*DeviceDef
}

// AccessMode is a definition of access mode bits.
type AccessMode uint

// Various access mode constants
const (
	ModeJoin    AccessMode = 1 << iota // user can join, i.e. {sub} (J:1)
	ModeRead                           // user can receive broadcasts ({data}, {info}) (R:2)
	ModeWrite                          // user can Write, i.e. {pub} (W:4)
	ModePres                           // user can receive presence updates (P:8)
	ModeApprove                        // user can approve new members or evict existing members (A:0x10)
	ModeShare                          // user can invite new members (S:0x20)
	ModeDelete                         // user can hard-delete messages (D:0x40)
	ModeOwner                          // user is the owner (O:0x80) - full access
	ModeUnset                          // Non-zero value to indicate unknown or undefined mode (:0x100),
	// to make it different from ModeNone

	ModeNone AccessMode = 0 // No access, requests to gain access are processed normally (N)

	// Normal user's access to a topic
	ModeCPublic AccessMode = ModeJoin | ModeRead | ModeWrite | ModePres | ModeShare
	// User's subscription to 'me' and 'fnd' - user can only read and delete incoming invites
	ModeCSelf AccessMode = ModeJoin | ModeRead | ModeDelete | ModePres
	// Owner's subscription to a generic topic
	ModeCFull AccessMode = ModeJoin | ModeRead | ModeWrite | ModePres | ModeApprove | ModeShare | ModeDelete | ModeOwner
	// Default P2P access mode
	ModeCP2P AccessMode = ModeJoin | ModeRead | ModeWrite | ModePres | ModeApprove
	// Read-only access to topic (0x3)
	ModeCReadOnly = ModeJoin | ModeRead

	// Admin: user who can modify access mode (hex: 0x90, dec: 144)
	ModeCAdmin = ModeOwner | ModeApprove
	// Sharer: flags which define user who can be notified of access mode changes (dec: 176, hex: 0xB0)
	ModeCSharer = ModeCAdmin | ModeShare

	// Invalid mode to indicate an error
	ModeInvalid AccessMode = 0x100000
)

// MarshalText converts AccessMode to string as byte slice.
func (m AccessMode) MarshalText() ([]byte, error) {

	// TODO: Need to distinguish between "not set" and "no access"
	if m == 0 {
		return []byte{'N'}, nil
	}

	if m == ModeInvalid {
		return nil, errors.New("AccessMode invalid")
	}

	var res = []byte{}
	var modes = []byte{'J', 'R', 'W', 'P', 'A', 'S', 'D', 'O'}
	for i, chr := range modes {
		if (m & (1 << uint(i))) != 0 {
			res = append(res, chr)
		}
	}
	return res, nil
}

// UnmarshalText parses access mode string as byte slice.
// Does not change the mode if the string is empty or invalid.
func (m *AccessMode) UnmarshalText(b []byte) error {
	m0 := ModeUnset

	for i := 0; i < len(b); i++ {
		switch b[i] {
		case 'J', 'j':
			m0 |= ModeJoin
		case 'R', 'r':
			m0 |= ModeRead
		case 'W', 'w':
			m0 |= ModeWrite
		case 'A', 'a':
			m0 |= ModeApprove
		case 'S', 's':
			m0 |= ModeShare
		case 'D', 'd':
			m0 |= ModeDelete
		case 'P', 'p':
			m0 |= ModePres
		case 'O', 'o':
			m0 |= ModeOwner
		case 'N', 'n':
			m0 = 0 // N means explicitly no access, all bits cleared
			break
		default:
			return errors.New("AccessMode: invalid character '" + string(b[i]) + "'")
		}
	}

	if m0 != ModeUnset {
		*m = m0
	}
	return nil
}

// String returns string representation of AccessMode.
func (m AccessMode) String() string {
	res, err := m.MarshalText()
	if err != nil {
		return ""
	}
	return string(res)
}

// MarshalJSON converts AccessMOde to quoted string.
func (m AccessMode) MarshalJSON() ([]byte, error) {
	res, err := m.MarshalText()
	if err != nil {
		return nil, err
	}

	res = append([]byte{'"'}, res...)
	return append(res, '"'), nil
}

// UnmarshalJSON reads AccessMode from a quoted string.
func (m *AccessMode) UnmarshalJSON(b []byte) error {
	if b[0] != '"' || b[len(b)-1] != '"' {
		return errors.New("syntax error")
	}

	return m.UnmarshalText(b[1 : len(b)-1])
}

// Scan is an implementation of sql.Scanner interface. It expects the
// value to be a byte slice representation of an ASCII string.
func (m *AccessMode) Scan(val interface{}) error {
	if bb, ok := val.([]byte); ok {
		return m.UnmarshalText(bb)
	}
	return errors.New("scan failed: data is not a byte slice")
}

// Value is an implementation of sql.driver.Valuer interface.
func (m AccessMode) Value() (driver.Value, error) {
	res, err := m.MarshalText()
	if err != nil {
		return "", err
	}
	return string(res), nil
}

// BetterEqual checks if grant mode allows all permissions requested in want mode.
func (grant AccessMode) BetterEqual(want AccessMode) bool {
	return grant&want == want
}

// Delta between two modes as a string old.Delta(new). JRPAS -> JRWS: "+W-PA"
// Zero delta is an empty string ""
func (o AccessMode) Delta(n AccessMode) string {

	// Removed bits, bits present in 'old' but missing in 'new' -> '-'
	o2n := o &^ n
	var removed string
	if o2n > 0 {
		removed = o2n.String()
		if removed != "" {
			removed = "-" + removed
		}
	}

	// Added bits, bits present in 'n' but missing in 'o' -> '+'
	n2o := n &^ o
	var added string
	if n2o > 0 {
		added = n2o.String()
		if added != "" {
			added = "+" + added
		}
	}
	return added + removed
}

// IsJoiner checks if joiner flag J is set.
func (m AccessMode) IsJoiner() bool {
	return m&ModeJoin != 0
}

// IsOwner checks if owner bit O is set.
func (m AccessMode) IsOwner() bool {
	return m&ModeOwner != 0
}

// IsApprover checks if approver A bit is set.
func (m AccessMode) IsApprover() bool {
	return m&ModeApprove != 0
}

// IsAdmin check if owner O or approver A flag is set.
func (m AccessMode) IsAdmin() bool {
	return m.IsOwner() || m.IsApprover()
}

// IsSharer checks if approver A or sharer S or owner O flag is set.
func (m AccessMode) IsSharer() bool {
	return m.IsAdmin() || (m&ModeShare != 0)
}

// IsWriter checks if allowed to publish (writer flag W is set).
func (m AccessMode) IsWriter() bool {
	return m&ModeWrite != 0
}

// IsReader checks if reader flag R is set.
func (m AccessMode) IsReader() bool {
	return m&ModeRead != 0
}

// IsPresencer checks if user receives presence updates (P flag set).
func (m AccessMode) IsPresencer() bool {
	return m&ModePres != 0
}

// IsDeleter checks if user can hard-delete messages (D flag is set).
func (m AccessMode) IsDeleter() bool {
	return m&ModeDelete != 0
}

// IsZero checks if no flags are set.
func (m AccessMode) IsZero() bool {
	return m == 0
}

// IsInvalid checks if mode is invalid.
func (m AccessMode) IsInvalid() bool {
	return m == ModeInvalid
}

// DefaultAccess is a per-topic default access modes
type DefaultAccess struct {
	Auth AccessMode
	Anon AccessMode
}

// Scan is an implementation of Scanner interface so the value can be read from SQL DBs
// It assumes the value is serialized and stored as JSON
func (da *DefaultAccess) Scan(val interface{}) error {
	return json.Unmarshal(val.([]byte), da)
}

// Value implements sql's driver.Valuer interface.
func (da DefaultAccess) Value() (driver.Value, error) {
	return json.Marshal(da)
}

// Subscription to a topic
type Subscription struct {
	ObjHeader
	// User who has relationship with the topic
	User string
	// Topic subscribed to
	Topic string

	// Subscription state, currently unused
	State int

	// Values persisted through subscription soft-deletion

	// ID of the latest Soft-delete operation
	DelId int
	// Last SeqId reported by user as received by at least one of his sessions
	RecvSeqId int
	// Last SeqID reported read by the user
	ReadSeqId int

	// Access mode requested by this user
	ModeWant AccessMode
	// Access mode granted to this user
	ModeGiven AccessMode
	// User's private data associated with the subscription to topic
	Private interface{}

	// Deserialized ephemeral values

	// Deserialized public value from topic or user (depends on context)
	// In case of P2P topics this is the Public value of the other user.
	public interface{}
	// deserialized SeqID from user or topic
	seqId int
	// Id of the last delete operation deserialized from user or topic
	// delId int
	// timestamp when the user was last online
	lastSeen time.Time
	// user agent string of the last online access
	userAgent string

	// P2P only. ID of the other user
	with string
	// P2P only. Default access: this is the mode given by the other user to this user
	modeDefault *DefaultAccess
}

// SetPublic assigns to public, otherwise not accessible from outside the package.
func (s *Subscription) SetPublic(pub interface{}) {
	s.public = pub
}

// GetPublic reads value of public.
func (s *Subscription) GetPublic() interface{} {
	return s.public
}

// SetWith sets other user for P2P subscriptions.
func (s *Subscription) SetWith(with string) {
	s.with = with
}

// GetWith returns the other user for P2P subscriptions.
func (s *Subscription) GetWith() string {
	return s.with
}

// GetSeqId returns seqId.
func (s *Subscription) GetSeqId() int {
	return s.seqId
}

// SetSeqId sets seqId field.
func (s *Subscription) SetSeqId(id int) {
	s.seqId = id
}

// GetLastSeen returns lastSeen.
func (s *Subscription) GetLastSeen() time.Time {
	return s.lastSeen
}

// GetUserAgent returns userAgent.
func (s *Subscription) GetUserAgent() string {
	return s.userAgent
}

// SetLastSeenAndUA updates lastSeen time and userAgent.
func (s *Subscription) SetLastSeenAndUA(when *time.Time, ua string) {
	if when != nil {
		s.lastSeen = *when
	}
	s.userAgent = ua
}

// SetDefaultAccess updates default access values.
func (s *Subscription) SetDefaultAccess(auth, anon AccessMode) {
	s.modeDefault = &DefaultAccess{auth, anon}
}

// GetDefaultAccess returns default access.
func (s *Subscription) GetDefaultAccess() *DefaultAccess {
	return s.modeDefault
}

// Contact is a result of a search for connections
type Contact struct {
	Id       string
	MatchOn  []string
	Access   DefaultAccess
	LastSeen time.Time
	Public   interface{}
}

type perUserData struct {
	private interface{}
	want    AccessMode
	given   AccessMode
}

// Topic stored in database
type Topic struct {
	ObjHeader

	// Name  string -- topic name is stored in Id

	// Use bearer token or use ACL
	UseBt bool

	// Default access to topic
	Access DefaultAccess

	// Server-issued sequential ID
	SeqId int
	// If messages were deleted, sequential id of the last operation to delete them
	DelId int

	Public interface{}

	// Indexed tags for finding this topic.
	Tags StringSlice

	// Deserialized ephemeral params
	owner   Uid                  // first assigned owner
	perUser map[Uid]*perUserData // deserialized from Subscription
}

// GiveAccess updates access mode for the given user.
func (t *Topic) GiveAccess(uid Uid, want AccessMode, given AccessMode) {
	if t.perUser == nil {
		t.perUser = make(map[Uid]*perUserData, 1)
	}

	pud := t.perUser[uid]
	if pud == nil {
		pud = &perUserData{}
	}

	pud.want = want
	pud.given = given

	t.perUser[uid] = pud
	if want&given&ModeOwner != 0 && t.owner.IsZero() {
		t.owner = uid
	}
}

// SetPrivate updates private value for the given user.
func (t *Topic) SetPrivate(uid Uid, private interface{}) {
	if t.perUser == nil {
		t.perUser = make(map[Uid]*perUserData, 1)
	}
	pud := t.perUser[uid]
	if pud == nil {
		pud = &perUserData{}
	}
	pud.private = private
	t.perUser[uid] = pud
}

// GetOwner returns topic's owner.
func (t *Topic) GetOwner() Uid {
	return t.owner
}

// GetPrivate returns given user's private value.
func (t *Topic) GetPrivate(uid Uid) (private interface{}) {
	if t.perUser == nil {
		return
	}
	pud := t.perUser[uid]
	if pud == nil {
		return
	}
	private = pud.private
	return
}

// GetAccess returns given user's access mode.
func (t *Topic) GetAccess(uid Uid) (mode AccessMode) {
	if t.perUser == nil {
		return
	}
	pud := t.perUser[uid]
	if pud == nil {
		return
	}
	mode = pud.given & pud.want
	return
}

// SoftDelete is a single DB record of soft-deletetion.
type SoftDelete struct {
	User  string
	DelId int
}

type MessageHeaders map[string]string

// Scan implements sql.Scanner interface.
func (mh *MessageHeaders) Scan(val interface{}) error {
	return json.Unmarshal(val.([]byte), mh)
}

// Value implements sql's driver.Valuer interface.
func (mh MessageHeaders) Value() (driver.Value, error) {
	return json.Marshal(mh)
}

// Message is a stored {data} message
type Message struct {
	ObjHeader
	// ID of the hard-delete operation
	DelId int `json:"DelId,omitempty"`
	// List of users who have marked this message as soft-deleted
	DeletedFor []SoftDelete `json:"DeletedFor,omitempty"`
	SeqId      int
	Topic      string
	// UID as string of the user who sent the message, could be empty
	From    string
	Head    MessageHeaders `json:"Head,omitempty"`
	Content interface{}
}

// Range is a range of message SeqIDs. If one ID in range, Hi is set to 0 or unset
type Range struct {
	Low int
	Hi  int `json:"Hi,omitempty"`
}

// RangeSorter is a helper type required by 'sort' package.
type RangeSorter []Range

// Len is the length of the range.
func (rs RangeSorter) Len() int {
	return len(rs)
}

// Swap swaps two items in a slice.
func (rs RangeSorter) Swap(i, j int) {
	rs[i], rs[j] = rs[j], rs[i]
}

// Less is a comparator. Sort by Low ascending, then sort by Hi descending
func (rs RangeSorter) Less(i, j int) bool {
	if rs[i].Low < rs[j].Low {
		return true
	} else if rs[i].Low == rs[j].Low {
		return rs[i].Hi >= rs[j].Hi
	}
	return false
}

// Normalize ranges - remove overlaps: [1..4],[2..4],[5..7] -> [1..7].
// The ranges are expected to be sorted.
// Ranges are inclusive-inclusive, i.e. [1..3] -> 1, 2, 3.
func (rs RangeSorter) Normalize() {
	ll := rs.Len()
	if ll > 1 {
		prev := 0
		for i := 1; i < ll; i++ {
			if rs[prev].Low == rs[i].Low {
				// Earlier range is guaranteed to be wider or equal to the later range,
				// collapse two ranges into one (by doing nothing)
				continue
			}
			// Check for full or partial overlap
			if rs[prev].Hi > 0 && rs[prev].Hi+1 >= rs[i].Low {
				// Partial overlap
				if rs[prev].Hi < rs[i].Hi {
					rs[prev].Hi = rs[i].Hi
				}
				// Otherwise the next range is fully within the previous range, consume it by doing nothing.
				continue
			}
			// No overlap
			prev++
		}
		rs = rs[:prev+1]
	}
}

// DelMessage is a log entry of a deleted message range.
type DelMessage struct {
	ObjHeader
	Topic       string
	DeletedFor  string
	DelId       int
	SeqIdRanges []Range
}

// BrowseOpt is an ID-based query, [since, before] - both ends inclusive (closed)
type BrowseOpt struct {
	Since  int
	Before int
	Limit  int
}

// TopicCat is an enum of topic categories.
type TopicCat int

const (
	// TopicCatMe is a value denoting 'me' topic.
	TopicCatMe TopicCat = iota
	// TopicCatFnd is a value denoting 'fnd' topic.
	TopicCatFnd
	// TopicCatP2P is a a value denoting 'p2p topic.
	TopicCatP2P
	// TopicCatGrp is a a value denoting group topic.
	TopicCatGrp
)

// GetTopicCat given topic name returns topic category.
func GetTopicCat(name string) TopicCat {
	switch name[:3] {
	case "usr":
		return TopicCatMe
	case "p2p":
		return TopicCatP2P
	case "grp":
		return TopicCatGrp
	case "fnd":
		return TopicCatFnd
	default:
		panic("invalid topic type for name '" + name + "'")
	}
}

// DeviceDef is the data provided by connected device. Used primarily for
// push notifications.
type DeviceDef struct {
	// Device registration ID
	DeviceId string
	// Device platform (iOS, Android, Web)
	Platform string
	// Last logged in
	LastSeen time.Time
	// Device language, ISO code
	Lang string
}
