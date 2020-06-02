// Package pin implements structures and methods to keep track of
// which objects a user wants to keep stored locally.
package pin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/dagutils"
	mdag "github.com/ipfs/go-merkledag"

	"github.com/TRON-US/go-btfs/logging"
	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ipld "github.com/ipfs/go-ipld-format"
)

const (
	DefaultDurationUnit  = time.Hour * 24
	DefaultDurationCount = 0
)

var log = logging.Logger("pin")
var recursivePinsKey = ds.NewKey("/local/pins/recursive/keys")
var directPinskey = ds.NewKey("/local/pins/direct/keys")

var pinDatastoreKey = ds.NewKey("/local/pins")

var emptyKey cid.Cid

func init() {
	e, err := cid.Decode("QmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n")
	if err != nil {
		log.Error("failed to decode empty key constant")
		os.Exit(1)
	}
	emptyKey = e
}

const (
	linkRecursive = "recursive"
	linkDirect    = "direct"
	linkIndirect  = "indirect"
	linkInternal  = "internal"
	linkNotPinned = "not pinned"
	linkAny       = "any"
	linkAll       = "all"
)

// Mode allows to specify different types of pin (recursive, direct etc.).
// See the Pin Modes constants for a full list.
type Mode int

// Pin Modes
const (
	// Recursive pins pin the target cids along with any reachable children.
	Recursive Mode = iota

	// Direct pins pin just the target cid.
	Direct

	// Indirect pins are cids who have some ancestor pinned recursively.
	Indirect

	// Internal pins are cids used to keep the internal state of the pinner.
	Internal

	// NotPinned
	NotPinned

	// Any refers to any pinned cid
	Any
)

// ModeToString returns a human-readable name for the Mode.
func ModeToString(mode Mode) (string, bool) {
	m := map[Mode]string{
		Recursive: linkRecursive,
		Direct:    linkDirect,
		Indirect:  linkIndirect,
		Internal:  linkInternal,
		NotPinned: linkNotPinned,
		Any:       linkAny,
	}
	s, ok := m[mode]
	return s, ok
}

// StringToMode parses the result of ModeToString() back to a Mode.
// It returns a boolean which is set to false if the mode is unknown.
func StringToMode(s string) (Mode, bool) {
	m := map[string]Mode{
		linkRecursive: Recursive,
		linkDirect:    Direct,
		linkIndirect:  Indirect,
		linkInternal:  Internal,
		linkNotPinned: NotPinned,
		linkAny:       Any,
		linkAll:       Any, // "all" and "any" means the same thing
	}
	mode, ok := m[s]
	return mode, ok
}

// A Pinner provides the necessary methods to keep track of Nodes which are
// to be kept locally, according to a pin mode. In practice, a Pinner is in
// in charge of keeping the list of items from the local storage that should
// not be garbage-collected.
type Pinner interface {
	// IsPinned returns whether or not the given cid is pinned
	// and an explanation of why its pinned
	IsPinned(cid.Cid) (string, bool, error)

	// IsPinnedWithType returns whether or not the given cid is pinned with the
	// given pin type, as well as returning the type of pin its pinned with.
	IsPinnedWithType(cid.Cid, Mode) (string, bool, error)

	// Pin the given node, optionally recursively.
	Pin(ctx context.Context, node ipld.Node, recursive bool, expir uint64) error

	// Unpin the given cid. If recursive is true, removes either a recursive or
	// a direct pin. If recursive is false, only removes a direct pin.
	Unpin(ctx context.Context, cid cid.Cid, recursive bool) error

	// Update updates a recursive pin from one cid to another
	// this is more efficient than simply pinning the new one and unpinning the
	// old one
	Update(ctx context.Context, from, to cid.Cid, unpin bool) error

	// Check if a set of keys are pinned, more efficient than
	// calling IsPinned for each key
	CheckIfPinned(cids ...cid.Cid) ([]Pinned, error)

	// PinWithMode is for manually editing the pin structure. Use with
	// care! If used improperly, garbage collection may not be
	// successful.
	PinWithMode(cid.Cid, uint64, Mode)

	// RemovePinWithMode is for manually editing the pin structure.
	// Use with care! If used improperly, garbage collection may not
	// be successful.
	RemovePinWithMode(cid.Cid, Mode)

	// Flush writes the pin state to the backing datastore
	Flush() error

	// DirectKeys returns all directly pinned cids
	DirectKeys() []cid.Cid

	// DirectMap returns all directly pinned cids and its values
	DirectMap() *cid.Map

	// DirectKeys returns all recursively pinned cids
	RecursiveKeys() []cid.Cid

	// RecursiveMap returns all recursively pinned cids and its values
	RecursiveMap() *cid.Map

	// InternalPins returns all cids kept pinned for the internal state of the
	// pinner
	InternalPins() []cid.Cid

	// HasExpiration returns true if the given Cid or its ancestor has
	// expiration time
	HasExpiration(cid.Cid) (bool, error)
}

// Pinned represents CID which has been pinned with a pinning strategy.
// The Via field allows to identify the pinning parent of this CID, in the
// case that the item is not pinned directly (but rather pinned recursively
// by some ascendant).
type Pinned struct {
	Key  cid.Cid
	Mode Mode
	Via  cid.Cid
}

// Pinned returns whether or not the given cid is pinned
func (p Pinned) Pinned() bool {
	return p.Mode != NotPinned
}

// String Returns pin status as string
func (p Pinned) String() string {
	switch p.Mode {
	case NotPinned:
		return "not pinned"
	case Indirect:
		return fmt.Sprintf("pinned via %s", p.Via)
	default:
		modeStr, _ := ModeToString(p.Mode)
		return fmt.Sprintf("pinned: %s", modeStr)
	}
}

// pinner implements the Pinner interface
type pinner struct {
	lock          sync.RWMutex
	recursePin    *cid.Set
	directPin     *cid.Set
	recursePinMap *cid.Map
	directPinMap  *cid.Map

	// Track the keys used for storing the pinning state, so gc does
	// not delete them.
	internalPin *cid.Set
	dserv       ipld.DAGService
	internal    ipld.DAGService // dagservice used to store internal objects
	dstore      ds.Datastore
}

// NewPinner creates a new pinner using the given datastore as a backend
func NewPinner(dstore ds.Datastore, serv, internal ipld.DAGService) Pinner {

	rcset := cid.NewSet()
	dirset := cid.NewSet()
	rcmap := cid.NewMap()
	dirmap := cid.NewMap()

	return &pinner{
		recursePin:    rcset,
		directPin:     dirset,
		recursePinMap: rcmap,
		directPinMap:  dirmap,
		dserv:         serv,
		dstore:        dstore,
		internal:      internal,
		internalPin:   cid.NewSet(),
	}
}

// Pin the given node, optionally recursive
func (p *pinner) Pin(ctx context.Context, node ipld.Node, recurse bool, expir uint64) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	err := p.dserv.Add(ctx, node)
	if err != nil {
		return err
	}

	c := node.Cid()

	if recurse {
		if p.recursePin.Has(c) && expir == 0 {
			return nil
		}

		if p.directPin.Has(c) {
			p.directPin.Remove(c)
		}
		p.lock.Unlock()
		// fetch entire graph
		err := mdag.FetchGraph(ctx, c, p.dserv)
		p.lock.Lock()
		if err != nil {
			return err
		}

		if p.recursePin.Has(c) && expir == 0 {
			return nil
		}

		if p.directPin.Has(c) {
			p.directPin.Remove(c)
		}

		p.recursePin.Add(c)
		if expir != 0 {
			v, found := p.recursePinMap.Get(c)
			if found {
				// If existing expiration time is later than the given `expir`, return
				if v.Expir >= expir {
					return nil
				}
			}
			p.recursePinMap.Add(c, expir)
		}
	} else {
		p.lock.Unlock()
		_, err := p.dserv.Get(ctx, c)
		p.lock.Lock()
		if err != nil {
			return err
		}

		if p.recursePin.Has(c) {
			return fmt.Errorf("%s already pinned recursively", c.String())
		}

		// Check if c is there and has expir. if yes, check existing expir and the given expir
		// Add only if existing expir < expir
		p.directPin.Add(c)
		if expir != 0 {
			v, found := p.directPinMap.Get(c)
			if found {
				// If existing expiration time is later than the given `expir`, return
				if v.Expir >= expir {
					return nil
				}
			}
			p.directPinMap.Add(c, expir)
		}

	}
	return nil
}

// ErrNotPinned is returned when trying to unpin items which are not pinned.
var ErrNotPinned = fmt.Errorf("not pinned or pinned indirectly")

// Unpin a given key
func (p *pinner) Unpin(ctx context.Context, c cid.Cid, recursive bool) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.recursePin.Has(c) {
		if !recursive {
			return fmt.Errorf("%s is pinned recursively", c)
		}
		p.recursePin.Remove(c)
		p.recursePinMap.Remove(c)
		return nil
	}
	if p.directPin.Has(c) {
		p.directPin.Remove(c)
		p.directPinMap.Remove(c)
		return nil
	}
	return ErrNotPinned
}

func (p *pinner) isInternalPin(c cid.Cid) bool {
	return p.internalPin.Has(c)
}

// IsPinned returns whether or not the given key is pinned
// and an explanation of why its pinned
func (p *pinner) IsPinned(c cid.Cid) (string, bool, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.isPinnedWithType(c, Any)
}

// IsPinnedWithType returns whether or not the given cid is pinned with the
// given pin type, as well as returning the type of pin its pinned with.
func (p *pinner) IsPinnedWithType(c cid.Cid, mode Mode) (string, bool, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.isPinnedWithType(c, mode)
}

// isPinnedWithType is the implementation of IsPinnedWithType that does not lock.
// intended for use by other pinned methods that already take locks.
func (p *pinner) isPinnedWithType(c cid.Cid, mode Mode) (string, bool, error) {
	switch mode {
	case Any, Direct, Indirect, Recursive, Internal:
	default:
		err := fmt.Errorf("invalid Pin Mode '%d', must be one of {%d, %d, %d, %d, %d}",
			mode, Direct, Indirect, Recursive, Internal, Any)
		return "", false, err
	}
	if (mode == Recursive || mode == Any) && p.recursePin.Has(c) {
		// Check if the given `c` is expired, if yes, return false.
		if p.recursePinMap.IsExpired(c) {
			return "", false, nil
		}
		return linkRecursive, true, nil
	}
	if mode == Recursive {
		return "", false, nil
	}

	if (mode == Direct || mode == Any) && p.directPin.Has(c) {
		if p.directPinMap.IsExpired(c) {
			return "", false, nil
		}
		return linkDirect, true, nil
	}
	if mode == Direct {
		return "", false, nil
	}

	if (mode == Internal || mode == Any) && p.isInternalPin(c) {
		return linkInternal, true, nil
	}
	if mode == Internal {
		return "", false, nil
	}

	// Default is Indirect
	visitedSet := cid.NewSet()
	for _, rc := range p.recursePin.Keys() {
		// If "rc" is expired, just skip. This will sufficiently satisfy the requirement
		// that a node should be kept until its last ancestor is still before expiration point in time.
		if p.recursePinMap.IsExpired(rc) {
			continue
		}
		has, err := hasChild(p.internal, rc, c, visitedSet.Visit)
		if err != nil {
			// Do not return error and skip bad children and continue the search
			log.Debug("unable to check pin on child [%s]: %v", rc.String(), err)
			continue
		}
		if has {
			return rc.String(), true, nil
		}
	}
	return "", false, nil
}

// CheckIfPinned Checks if a set of keys are pinned, more efficient than
// calling IsPinned for each key, returns the pinned status of cid(s)
func (p *pinner) CheckIfPinned(cids ...cid.Cid) ([]Pinned, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	pinned := make([]Pinned, 0, len(cids))
	toCheck := cid.NewSet()

	// First check for non-Indirect pins directly
	for _, c := range cids {
		if p.recursePin.Has(c) {
			if p.recursePinMap.IsExpired(c) {
				continue
			}
			pinned = append(pinned, Pinned{Key: c, Mode: Recursive})
		} else if p.directPin.Has(c) {
			if p.directPinMap.IsExpired(c) {
				continue
			}
			pinned = append(pinned, Pinned{Key: c, Mode: Direct})
		} else if p.isInternalPin(c) {
			pinned = append(pinned, Pinned{Key: c, Mode: Internal})
		} else {
			toCheck.Add(c)
		}
	}

	// Now walk all recursive pins to check for indirect pins
	var checkChildren func(cid.Cid, cid.Cid) error
	checkChildren = func(rk, parentKey cid.Cid) error {
		links, err := ipld.GetLinks(context.TODO(), p.dserv, parentKey)
		if err != nil {
			return err
		}
		for _, lnk := range links {
			c := lnk.Cid

			if toCheck.Has(c) {
				pinned = append(pinned,
					Pinned{Key: c, Mode: Indirect, Via: rk})
				toCheck.Remove(c)
			}

			err := checkChildren(rk, c)
			if err != nil {
				return err
			}

			if toCheck.Len() == 0 {
				return nil
			}
		}
		return nil
	}

	for _, rk := range p.recursePin.Keys() {
		if p.recursePinMap.IsExpired(rk) {
			continue
		}
		err := checkChildren(rk, rk)
		if err != nil {
			return nil, err
		}
		if toCheck.Len() == 0 {
			break
		}
	}

	// Anything left in toCheck is not pinned
	for _, k := range toCheck.Keys() {
		pinned = append(pinned, Pinned{Key: k, Mode: NotPinned})
	}

	return pinned, nil
}

// RemovePinWithMode is for manually editing the pin structure.
// Use with care! If used improperly, garbage collection may not
// be successful.
func (p *pinner) RemovePinWithMode(c cid.Cid, mode Mode) {
	p.lock.Lock()
	defer p.lock.Unlock()
	switch mode {
	case Direct:
		p.directPin.Remove(c)
	case Recursive:
		p.recursePin.Remove(c)
	default:
		// programmer error, panic OK
		panic("unrecognized pin type")
	}
}

func cidSetWithValues(cids []cid.Cid) *cid.Set {
	out := cid.NewSet()
	for _, c := range cids {
		out.Add(c)
	}
	return out
}

// LoadPinner loads a pinner and its keysets from the given datastore
func LoadPinner(d ds.Datastore, dserv, internal ipld.DAGService) (Pinner, error) {
	p := new(pinner)

	rootKey, err := d.Get(pinDatastoreKey)
	if err != nil {
		return nil, fmt.Errorf("cannot load pin state: %v", err)
	}
	rootCid, err := cid.Cast(rootKey)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*5)
	defer cancel()

	root, err := internal.Get(ctx, rootCid)
	if err != nil {
		return nil, fmt.Errorf("cannot find pinning root object: %v", err)
	}

	rootpb, ok := root.(*mdag.ProtoNode)
	if !ok {
		return nil, mdag.ErrNotProtobuf
	}

	internalset := cid.NewSet()
	internalset.Add(rootCid)
	recordInternal := internalset.Add

	{ // load recursive set and map
		recurseKeys, err := loadSet(ctx, internal, rootpb, linkRecursive, recordInternal)
		if err != nil {
			return nil, fmt.Errorf("cannot load recursive pins: %v", err)
		}
		p.recursePin = cidSetWithValues(recurseKeys)

		p.recursePinMap, err = LoadMap(d, recursivePinsKey)
		if err != nil {
			return nil, fmt.Errorf("cannot load recursive pins' map: %v", err)
		}
	}

	{ // load direct set and map
		directKeys, err := loadSet(ctx, internal, rootpb, linkDirect, recordInternal)
		if err != nil {
			return nil, fmt.Errorf("cannot load direct pins: %v", err)
		}
		p.directPin = cidSetWithValues(directKeys)

		p.directPinMap, err = LoadMap(d, directPinskey)
		if err != nil {
			return nil, fmt.Errorf("cannot load direct pins' map: %v", err)
		}
	}

	p.internalPin = internalset

	// assign services
	p.dserv = dserv
	p.dstore = d
	p.internal = internal

	return p, nil
}

// DirectKeys returns a slice containing the directly pinned keys
func (p *pinner) DirectKeys() []cid.Cid {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.directPin.Keys()
}

// DirectMap returns the map for the directly pinned keys
func (p *pinner) DirectMap() *cid.Map {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.directPinMap
}

// RecursiveKeys returns a slice containing the recursively pinned keys
func (p *pinner) RecursiveKeys() []cid.Cid {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.recursePin.Keys()
}

// RecursiveMap returns the map for the recursively pinned keys
func (p *pinner) RecursiveMap() *cid.Map {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.recursePinMap
}

// Update updates a recursive pin from one cid to another
// this is more efficient than simply pinning the new one and unpinning the
// old one
func (p *pinner) Update(ctx context.Context, from, to cid.Cid, unpin bool) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.recursePin.Has(from) {
		return fmt.Errorf("'from' cid was not recursively pinned already")
	}

	err := dagutils.DiffEnumerate(ctx, p.dserv, from, to)
	if err != nil {
		return err
	}

	p.recursePin.Add(to)
	if unpin {
		p.recursePin.Remove(from)
	}
	return nil
}

// Flush encodes and writes pinner keysets to the datastore
func (p *pinner) Flush() error {
	p.lock.Lock()
	defer p.lock.Unlock()

	ctx := context.TODO()

	internalset := cid.NewSet()
	recordInternal := internalset.Add

	root := &mdag.ProtoNode{}
	{
		n, err := storeSet(ctx, p.internal, p.directPin.Keys(), p.directPin, p.directPinMap, recordInternal)
		if err != nil {
			return err
		}
		if err := root.AddNodeLink(linkDirect, n); err != nil {
			return err
		}
		err = storeMap(p.dstore, directPinskey, p.directPinMap)
		if err != nil {
			return err
		}
	}

	{
		n, err := storeSet(ctx, p.internal, p.recursePin.Keys(), p.recursePin, p.recursePinMap, recordInternal)
		if err != nil {
			return err
		}
		if err := root.AddNodeLink(linkRecursive, n); err != nil {
			return err
		}
		err = storeMap(p.dstore, recursivePinsKey, p.recursePinMap)
		if err != nil {
			return err
		}
	}

	// add the empty node, its referenced by the pin sets but never created
	err := p.internal.Add(ctx, new(mdag.ProtoNode))
	if err != nil {
		return err
	}

	err = p.internal.Add(ctx, root)
	if err != nil {
		return err
	}

	k := root.Cid()

	internalset.Add(k)
	if err := p.dstore.Put(pinDatastoreKey, k.Bytes()); err != nil {
		return fmt.Errorf("cannot store pin state: %v", err)
	}
	p.internalPin = internalset
	return nil
}

// InternalPins returns all cids kept pinned for the internal state of the
// pinner
func (p *pinner) InternalPins() []cid.Cid {
	p.lock.Lock()
	defer p.lock.Unlock()
	var out []cid.Cid
	out = append(out, p.internalPin.Keys()...)
	return out
}

// PinWithMode allows the user to have fine grained control over pin
// counts
func (p *pinner) PinWithMode(c cid.Cid, expir uint64, mode Mode) {
	p.lock.Lock()
	defer p.lock.Unlock()
	switch mode {
	case Recursive:
		p.recursePin.Add(c)
		p.recursePinMap.Add(c, expir)
	case Direct:
		p.directPin.Add(c)
		p.directPinMap.Add(c, expir)
	}
}

func (p *pinner) HasExpiration(c cid.Cid) (bool, error) {
	if p.directPin.Has(c) {
		if p.directPinMap.HasExpiration(c) {
			return true, nil
		} else {
			return false, nil
		}
	}

	if p.recursePin.Has(c) && p.recursePinMap.HasExpiration(c) {
		return true, nil
	}

	visitedSet := cid.NewSet()
	for _, k := range p.recursePin.Keys() {
		if !p.recursePinMap.HasExpiration(k) {
			continue
		}
		has, err := hasChild(p.internal, k, c, visitedSet.Visit)
		if err != nil {
			// Do not return error and skip bad children and continue the search
			log.Debug("unable to check expiration on child [%s]: %v", k.String(), err)
			continue
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

func IsExpiredPin(c cid.Cid, pinRecurMap *cid.Map, pinDirectMap *cid.Map) bool {
	if pinRecurMap != nil && pinRecurMap.IsExpired(c) {
		return true
	}
	if pinDirectMap != nil && pinDirectMap.IsExpired(c) {
		return true
	}
	return false
}

// hasChild recursively looks for a Cid among the children of a root Cid.
// The visit function can be used to shortcut already-visited branches.
func hasChild(ng ipld.NodeGetter, root cid.Cid, child cid.Cid, visit func(cid.Cid) bool) (bool, error) {
	links, err := ipld.GetLinks(context.TODO(), ng, root)
	if err != nil {
		return false, err
	}
	for _, lnk := range links {
		c := lnk.Cid
		if lnk.Cid.Equals(child) {
			return true, nil
		}
		if visit(c) {
			has, err := hasChild(ng, c, child, visit)
			if err != nil {
				return false, err
			}

			if has {
				return has, nil
			}
		}
	}
	return false, nil
}

// Returns time.Unix value for the given `durationUnit` and `durationCount` from now.
func ExpiresAtWithUnitAndCount(durationUnit time.Duration, durationCount int64) (uint64, error) {
	if durationUnit == 0 || durationCount == 0 {
		return 0, nil
	}
	timeDuration := time.Duration(durationUnit) * time.Duration(durationCount)
	return ExpiresAt(timeDuration), nil
}

// Returns time.Unix value for the given `dur` from now.
func ExpiresAt(dur time.Duration) uint64 {
	// TODO: check overflow!
	return uint64(time.Now().Add(dur).Unix())
}

func storeMap(d ds.Datastore, k ds.Key, v *cid.Map) error {
	m := make(map[string]cid.Value)
	for ke, va := range v.CidMap {
		m[ke.String()] = va
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return d.Put(k, b)
}

func LoadMap(d ds.Datastore, k ds.Key) (*cid.Map, error) {
	val, err := d.Get(k)
	if err != nil {
		return nil, err
	}

	v := cid.NewMap()
	m := make(map[string]cid.Value)
	err = json.Unmarshal([]byte(val), &m)
	if err != nil {
		return nil, err
	}
	for ke, va := range m {
		k, err := cid.Parse(ke)
		if err != nil {
			return nil, err
		}
		v.CidMap[k] = va
	}
	return v, nil
}
