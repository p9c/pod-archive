package waddrmgr

import (
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"github.com/p9c/log"
	"github.com/p9c/pod/pkg/btcaddr"
	"github.com/p9c/pod/pkg/chaincfg"
	"sync"
	"time"
	
	"github.com/p9c/pod/pkg/snacl"
	"github.com/p9c/pod/pkg/util/hdkeychain"
	"github.com/p9c/pod/pkg/util/zero"
	"github.com/p9c/pod/pkg/walletdb"
)

const (
	// MaxAccountNum is the maximum allowed account number. This value was chosen
	// because accounts are hardened children and therefore must not exceed the
	// hardened child range of extended keys and it provides a reserved account at
	// the top of the range for supporting imported addresses.
	MaxAccountNum = hdkeychain.HardenedKeyStart - 2 // 2^31 - 2
	// MaxAddressesPerAccount is the maximum allowed number of addresses per account
	// number. This value is based on the limitation of the underlying hierarchical
	// deterministic key derivation.
	MaxAddressesPerAccount = hdkeychain.HardenedKeyStart - 1
	// ImportedAddrAccount is the account number to use for all imported addresses.
	// This is useful since normal accounts are derived from the root hierarchical
	// deterministic key and imported addresses do not fit into that model.
	ImportedAddrAccount = MaxAccountNum + 1 // 2^31 - 1
	// ImportedAddrAccountName is the name of the imported account.
	ImportedAddrAccountName = "imported"
	// DefaultAccountNum is the number of the default account.
	DefaultAccountNum = 0
	// defaultAccountName is the initial name of the default account. Note that the
	// default account may be renamed and is not a reserved name, so the default
	// account might not be named "default" and non-default accounts may be named
	// "default".
	//
	// Account numbers never change, so the DefaultAccountNum should be used to
	// refer to (and only to) the default account.
	defaultAccountName = "default"
	// The hierarchy described by BIP0043 is:
	//
	//  m/<purpose>'/*
	//
	// This is further extended by BIP0044 to:
	//
	//  m/44'/<coin type>'/<account>'/<branch>/<address index>
	//
	// The branch is 0 for external addresses and 1 for internal addresses.
	// maxCoinType is the maximum allowed coin type used when structuring the
	// BIP0044 multi-account hierarchy. This value is based on the limitation of the
	// underlying hierarchical deterministic key derivation.
	maxCoinType = hdkeychain.HardenedKeyStart - 1
	// ExternalBranch is the child number to use when performing BIP0044 style
	// hierarchical deterministic key derivation for the external branch.
	ExternalBranch uint32 = 0
	// InternalBranch is the child number to use when performing BIP0044 style
	// hierarchical deterministic key derivation for the internal branch.
	InternalBranch uint32 = 1
	// saltSize is the number of bytes of the salt used when hashing private
	// passphrases.
	saltSize = 32
)

// isReservedAccountName returns true if the account name is reserved. Reserved
// accounts may never be renamed, and other accounts may not be renamed to a
// reserved name.
func isReservedAccountName(name string) bool {
	return name == ImportedAddrAccountName
}

// isReservedAccountNum returns true if the account number is reserved. Reserved
// accounts may not be renamed.
func isReservedAccountNum(acct uint32) bool {
	return acct == ImportedAddrAccount
}

// ScryptOptions is used to hold the scrypt parameters needed when deriving new
// passphrase keys.
type ScryptOptions struct {
	N, R, P int
}

// OpenCallbacks houses caller-provided callbacks that may be called when
// opening an existing manager. The open blocks on the execution of these
// functions.
type OpenCallbacks struct {
	// ObtainSeed is a callback function that is potentially invoked during
	// upgrades. It is intended to be used to request the wallet seed from the user
	// (or any other mechanism the caller deems fit).
	ObtainSeed ObtainUserInputFunc
	// ObtainPrivatePass is a callback function that is potentially invoked during
	// upgrades. It is intended to be used to request the wallet private passphrase
	// from the user (or any other mechanism the caller deems fit).
	ObtainPrivatePass ObtainUserInputFunc
}

// DefaultScryptOptions is the default options used with scrypt.
var DefaultScryptOptions = ScryptOptions{
	N: 262144, // 2^18
	R: 8,
	P: 1,
}

// addrKey is used to uniquely identify an address even when those addresses
// would end up being the same bitcoin address (as is the case for pay-to-pubkey
// and pay-to-pubkey-hash style of addresses).
type addrKey string

// accountInfo houses the current state of the internal and external branches of
// an account along with the extended keys needed to derive new keys. It also
// handles locking by keeping an encrypted version of the serialized private
// extended key so the unencrypted versions can be cleared from memory when the
// address manager is locked.
type accountInfo struct {
	acctName string
	// The account key is used to derive the branches which in turn derive the
	// internal and external addresses. The accountKeyPriv will be nil when the
	// address manager is locked.
	acctKeyEncrypted []byte
	acctKeyPriv      *hdkeychain.ExtendedKey
	acctKeyPub       *hdkeychain.ExtendedKey
	lastExternalAddr ManagedAddress
	lastInternalAddr ManagedAddress
	// The external branch is used for all addresses which are intended for external
	// use.
	nextExternalIndex uint32
	// The internal branch is used for all adddresses which are only intended for
	// internal wallet use such as change addresses.
	nextInternalIndex uint32
}

// AccountProperties contains properties associated with each account, such as
// the account name, number, and the nubmer of derived and imported keys.
type AccountProperties struct {
	AccountName      string
	AccountNumber    uint32
	ExternalKeyCount uint32
	InternalKeyCount uint32
	ImportedKeyCount uint32
}

// unlockDeriveInfo houses the information needed to derive a private key for a
// managed address when the address manager is unlocked. See the deriveOnUnlock
// field in the Manager struct for more details on how this is used.
type unlockDeriveInfo struct {
	managedAddr ManagedAddress
	branch      uint32
	index       uint32
}

// SecretKeyGenerator is the function signature of a method that can generate
// secret keys for the address manager.
type SecretKeyGenerator func(
	passphrase *[]byte, config *ScryptOptions,
) (*snacl.SecretKey, error)

// defaultNewSecretKey returns a new secret key.  See newSecretKey.
func defaultNewSecretKey(
	passphrase *[]byte,
	config *ScryptOptions,
) (*snacl.SecretKey, error) {
	return snacl.NewSecretKey(passphrase, config.N, config.R, config.P)
}

var (
	// secretKeyGen is the inner method that is executed when calling newSecretKey.
	secretKeyGen = defaultNewSecretKey
	// secretKeyGenMtx protects access to secretKeyGen, so that it can be replaced
	// in testing.
	secretKeyGenMtx sync.RWMutex
)

// SetSecretKeyGen replaces the existing secret key generator, and returns the
// previous generator.
func SetSecretKeyGen(keyGen SecretKeyGenerator) SecretKeyGenerator {
	secretKeyGenMtx.Lock()
	oldKeyGen := secretKeyGen
	secretKeyGen = keyGen
	secretKeyGenMtx.Unlock()
	return oldKeyGen
}

// newSecretKey generates a new secret key using the active secretKeyGen.
func newSecretKey(passphrase *[]byte, config *ScryptOptions) (*snacl.SecretKey, error) {
	secretKeyGenMtx.RLock()
	defer secretKeyGenMtx.RUnlock()
	return secretKeyGen(passphrase, config)
}

// EncryptorDecryptor provides an abstraction on top of snacl.CryptoKey so that
// our tests can use dependency injection to force the behaviour they need.
type EncryptorDecryptor interface {
	Encrypt(in []byte) ([]byte, error)
	Decrypt(in []byte) ([]byte, error)
	Bytes() []byte
	CopyBytes([]byte)
	Zero()
}

// cryptoKey extends snacl.CryptoKey to implement EncryptorDecryptor.
type cryptoKey struct {
	snacl.CryptoKey
}

// Bytes returns a copy of this crypto key's byte slice.
func (ck *cryptoKey) Bytes() []byte {
	return ck.CryptoKey[:]
}

// CopyBytes copies the bytes from the given slice into this CryptoKey.
func (ck *cryptoKey) CopyBytes(from []byte) {
	copy(ck.CryptoKey[:], from)
}

// defaultNewCryptoKey returns a new CryptoKey.  See newCryptoKey.
func defaultNewCryptoKey() (EncryptorDecryptor, error) {
	var key *snacl.CryptoKey
	var e error
	if key, e = snacl.GenerateCryptoKey(); E.Chk(e) {
		return nil, e
	}
	return &cryptoKey{*key}, nil
}

// CryptoKeyType is used to differentiate between different kinds of crypto
// keys.
type CryptoKeyType byte

// Crypto key types.
const (
	// CKTPrivate specifies the key that is used for encryption of private key
	// material such as derived extended private keys and imported private keys.
	CKTPrivate CryptoKeyType = iota
	// CKTScript specifies the key that is used for encryption of scripts.
	CKTScript
	// CKTPublic specifies the key that is used for encryption of public key
	// material such as dervied extended public keys and imported public keys.
	CKTPublic
)

// newCryptoKey is used as a way to replace the new crypto key generation
// function used so tests can provide a version that fails for testing error
// paths.
var newCryptoKey = defaultNewCryptoKey

// Manager represents a concurrency safe crypto currency address manager and key
// store.
type Manager struct {
	mtx sync.RWMutex
	// scopedManager is a mapping of scope of scoped manager, the manager itself
	// loaded into memory.
	scopedManagers      map[KeyScope]*ScopedKeyManager
	externalAddrSchemas map[AddressType][]KeyScope
	internalAddrSchemas map[AddressType][]KeyScope
	syncState           syncState
	birthday            time.Time
	chainParams         *chaincfg.Params
	// masterKeyPub is the secret key used to secure the cryptoKeyPub key and
	// masterKeyPriv is the secret key used to secure the cryptoKeyPriv key. This
	// approach is used because it makes changing the passwords much simpler as it
	// then becomes just changing these keys. It also provides future flexibility.
	//
	// NOTE: This is not the same thing as BIP0032 master node extended key.
	//
	// The underlying master private key will be zeroed when the address manager is
	// locked.
	masterKeyPub  *snacl.SecretKey
	masterKeyPriv *snacl.SecretKey
	// cryptoKeyPub is the key used to encrypt public extended keys and addresses.
	cryptoKeyPub EncryptorDecryptor
	// cryptoKeyPriv is the key used to encrypt private data such as the master
	// hierarchical deterministic extended key.
	//
	// This key will be zeroed when the address manager is locked.
	cryptoKeyPrivEncrypted []byte
	cryptoKeyPriv          EncryptorDecryptor
	// cryptoKeyScript is the key used to encrypt script data.
	//
	// This key will be zeroed when the address manager is locked.
	cryptoKeyScriptEncrypted []byte
	cryptoKeyScript          EncryptorDecryptor
	// privPassphraseSalt and hashedPrivPassphrase allow for the secure detection of
	// a correct passphrase on manager unlock when the manager is already unlocked.
	// The hash is zeroed each lock.
	privPassphraseSalt   [saltSize]byte
	hashedPrivPassphrase [sha512.Size]byte
	watchingOnly         bool
	locked               bool
	closed               bool
}

// WatchOnly returns true if the root manager is in watch only mode, and false otherwise.
func (m *Manager) WatchOnly() bool {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return m.watchingOnly
}

// lock performs a best try effort to remove and zero all secret keys associated
// with the address manager.
//
// This function MUST be called with the manager lock held for writes.
func (m *Manager) lock() {
	for _, manager := range m.scopedManagers {
		// Clear all of the account private keys.
		for _, acctInfo := range manager.acctInfo {
			if acctInfo.acctKeyPriv != nil {
				acctInfo.acctKeyPriv.Zero()
			}
			acctInfo.acctKeyPriv = nil
		}
	}
	// Remove clear text private keys and scripts from all address entries.
	for _, manager := range m.scopedManagers {
		for _, ma := range manager.addrs {
			switch addr := ma.(type) {
			case *managedAddress:
				addr.lock()
			case *scriptAddress:
				addr.lock()
			}
		}
	}
	// Remove clear text private master and crypto keys from memory.
	m.cryptoKeyScript.Zero()
	m.cryptoKeyPriv.Zero()
	m.masterKeyPriv.Zero()
	// Zero the hashed passphrase.
	zero.Bytea64(&m.hashedPrivPassphrase)
	// NOTE: m.cryptoKeyPub is intentionally not cleared here as the address manager
	// needs to be able to continue to read and decrypt public data which uses a
	// separate derived key from the database even when it is locked.
	m.locked = true
}

// Close cleanly shuts down the manager. It makes a best try effort to remove
// and zero all private key and sensitive public key material associated with
// the address manager from memory.
func (m *Manager) Close() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.closed {
		return
	}
	for _, manager := range m.scopedManagers {
		// Zero out the account keys (if any) of all sub key managers.
		manager.Close()
	}
	// Attempt to clear private key material from memory.
	if !m.watchingOnly && !m.locked {
		m.lock()
	}
	// Remove clear text public master and crypto keys from memory.
	m.cryptoKeyPub.Zero()
	m.masterKeyPub.Zero()
	m.closed = true
	// return
}

// NewScopedKeyManager creates a new scoped key manager from the root manager. A
// scoped key manager is a sub-manager that only has the coin type key of a
// particular coin type and BIP0043 purpose. This is useful as it enables
// callers to create an arbitrary BIP0043 like schema with a stand alone
// manager.
//
// Note that a new scoped manager cannot be created if: the wallet is watch
// only, the manager hasn't been unlocked, or the root key has been. neutered
// from the database.
//
// TODO(roasbeef): addrtype of raw key means it'll look in scripts to possibly mark as gucci?
func (m *Manager) NewScopedKeyManager(
	ns walletdb.ReadWriteBucket, scope KeyScope,
	addrSchema ScopeAddrSchema,
) (*ScopedKeyManager, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// If the manager is locked, then we can't create a new scoped manager.
	if m.locked {
		return nil, managerError(ErrLocked, errLocked, nil)
	}
	// Now that we know the manager is unlocked, we'll need to fetch the root master
	// HD private key. This is required as we'll be attempting the following
	// derivation: m/purpose'/cointype'
	//
	// Note that the path to the coin type is requires hardened derivation,
	// therefore this can only be done if the wallet's root key hasn't been
	// neutered.
	var masterRootPrivEnc []byte
	var e error
	if masterRootPrivEnc, _, e = fetchMasterHDKeys(ns); E.Chk(e) {
		return nil, e
	}
	// If the master root private key isn't found within the database, but we need
	// to bail here as we can't create the cointype key without the master root
	// private key.
	if masterRootPrivEnc == nil {
		return nil, managerError(ErrWatchingOnly, "", nil)
	}
	// Before we can derive any new scoped managers using this key, we'll need to
	// fully decrypt it.
	var serializedMasterRootPriv []byte
	if serializedMasterRootPriv, e = m.cryptoKeyPriv.Decrypt(masterRootPrivEnc); E.Chk(e) {
		str := fmt.Sprintf("failed to decrypt master root serialized private key")
		return nil, managerError(ErrLocked, str, e)
	}
	// Now that we know the root priv is within the database, we'll decode it into a
	// usable object.
	var rootPriv *hdkeychain.ExtendedKey
	if rootPriv, e = hdkeychain.NewKeyFromString(
		string(serializedMasterRootPriv),
	); E.Chk(e) {
		str := fmt.Sprintf("failed to create master extended private key")
		zero.Bytes(serializedMasterRootPriv)
		return nil, managerError(ErrKeyChain, str, e)
	}
	zero.Bytes(serializedMasterRootPriv)
	// Now that we have the root private key, we'll fetch the scope bucket so we can
	// create the proper internal name spaces.
	scopeBucket := ns.NestedReadWriteBucket(scopeBucketName)
	// Now that we know it's possible to actually create a new scoped manager, we'll
	// carve out its bucket space within the database.
	if e = createScopedManagerNS(scopeBucket, &scope); E.Chk(e) {
		return nil, e
	}
	// With the database state created, we'll now write down the address schema of
	// this particular scope type.
	scopeSchemas := ns.NestedReadWriteBucket(scopeSchemaBucketName)
	if scopeSchemas == nil {
		str := "scope schema bucket not found"
		return nil, managerError(ErrDatabase, str, nil)
	}
	scopeKey := scopeToBytes(&scope)
	schemaBytes := scopeSchemaToBytes(&addrSchema)
	if e = scopeSchemas.Put(scopeKey[:], schemaBytes); E.Chk(e) {
		return nil, e
	}
	// With the database state created, we'll now derive the cointype key using the
	// master HD private key, then encrypt it along with the first account using our
	// crypto keys.
	if e = createManagerKeyScope(
		ns, scope, rootPriv, m.cryptoKeyPub, m.cryptoKeyPriv,
	); E.Chk(e) {
		return nil, e
	}
	// Finally, we'll register this new scoped manager with the root manager.
	m.scopedManagers[scope] = &ScopedKeyManager{
		scope:       scope,
		addrSchema:  addrSchema,
		rootManager: m,
		addrs:       make(map[addrKey]ManagedAddress),
		acctInfo:    make(map[uint32]*accountInfo),
	}
	m.externalAddrSchemas[addrSchema.ExternalAddrType] = append(
		m.externalAddrSchemas[addrSchema.ExternalAddrType], scope,
	)
	m.internalAddrSchemas[addrSchema.InternalAddrType] = append(
		m.internalAddrSchemas[addrSchema.InternalAddrType], scope,
	)
	return m.scopedManagers[scope], nil
}

// FetchScopedKeyManager attempts to fetch an active scoped manager according to
// its registered scope. If the manger is found, then a nil error is returned
// along with the active scoped manager. Otherwise, a nil manager and a non-nil
// error will be returned.
func (m *Manager) FetchScopedKeyManager(scope KeyScope) (*ScopedKeyManager, error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	sm, ok := m.scopedManagers[scope]
	if !ok {
		str := fmt.Sprintf("scope %v not found", scope)
		return nil, managerError(ErrScopeNotFound, str, nil)
	}
	return sm, nil
}

// ActiveScopedKeyManagers returns a slice of all the active scoped key managers
// currently known by the root key manager.
func (m *Manager) ActiveScopedKeyManagers() []*ScopedKeyManager {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	scopedManagers := make([]*ScopedKeyManager, len(m.scopedManagers))
	for _, smgr := range m.scopedManagers {
		scopedManagers = append(scopedManagers, smgr)
	}
	return scopedManagers
}

// ScopesForExternalAddrType returns the set of key scopes that are able to
// produce the target address type as external addresses.
func (m *Manager) ScopesForExternalAddrType(addrType AddressType) []KeyScope {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return m.externalAddrSchemas[addrType]
}

// ScopesForInternalAddrTypes returns the set of key scopes that are able to
// produce the target address type as internal addresses.
func (m *Manager) ScopesForInternalAddrTypes(addrType AddressType) []KeyScope {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return m.internalAddrSchemas[addrType]
}

// NeuterRootKey is a special method that should be used once a caller is
// *certain* that no further scoped managers are to be created. This method will
// *delete* the encrypted master HD root private key from the database.
func (m *Manager) NeuterRootKey(ns walletdb.ReadWriteBucket) (e error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// First, we'll fetch the current master HD keys from the database.
	var masterRootPrivEnc []byte
	if masterRootPrivEnc, _, e = fetchMasterHDKeys(ns); E.Chk(e) {
		return e
	}
	// If the root master private key is already nil, then we'll return a nil error
	// here as the root key has already been permanently neutered.
	if masterRootPrivEnc == nil {
		return nil
	}
	zero.Bytes(masterRootPrivEnc)
	// Otherwise, we'll neuter the root key permanently by deleting the encrypted
	// master HD key from the database.
	return ns.NestedReadWriteBucket(mainBucketName).Delete(masterHDPrivName)
}

// Address returns a managed address given the passed address if it is known to
// the address manager. A managed address differs from the passed address in
// that it also potentially contains extra information needed to sign
// transactions such as the associated private key for pay-to-pubkey and
// pay-to-pubkey-hash addresses and the script associated with
// pay-to-script-hash addresses.
func (m *Manager) Address(
	ns walletdb.ReadBucket,
	address btcaddr.Address,
) (ManagedAddress, error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	// We'll iterate through each of the known scoped managers, and see if any of them now of the target address.
	for _, scopedMgr := range m.scopedManagers {
		addr, e := scopedMgr.Address(ns, address)
		if e != nil {
			continue
		}
		return addr, nil
	}
	// If the address wasn't known to any of the scoped managers, then we'll return an error.
	str := fmt.Sprintf("unable to find key for addr %v", address)
	return nil, managerError(ErrAddressNotFound, str, nil)
}

// MarkUsed updates the used flag for the provided address.
func (m *Manager) MarkUsed(ns walletdb.ReadWriteBucket, address btcaddr.Address) (e error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	// Run through all the known scoped managers, and attempt to mark the address as
	// used for each one. First, we'll figure out which scoped manager this address
	// belong to.
	for _, scopedMgr := range m.scopedManagers {
		if _, e = scopedMgr.Address(ns, address); E.Chk(e) {
			continue
		}
		// We've found the manager that this address belongs to, so we can mark the
		// address as used and return.
		return scopedMgr.MarkUsed(ns, address)
	}
	// If we get to this point, then we weren't able to find the address in any of
	// the managers, so we'll exit with an error.
	str := fmt.Sprintf("unable to find key for addr %v", address)
	return managerError(ErrAddressNotFound, str, nil)
}

// AddrAccount returns the account to which the given address belongs. We also
// return the scoped manager that owns the addr+account combo.
func (m *Manager) AddrAccount(
	ns walletdb.ReadBucket,
	address btcaddr.Address,
) (*ScopedKeyManager, uint32, error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	var e error
	for _, scopedMgr := range m.scopedManagers {
		if _, e = scopedMgr.Address(ns, address); e != nil /*T.Chk(e)*/ {
			// D.Ln(address)
			continue
		}
		// We've found the manager that this address belongs to, so we can retrieve the
		// address' account along with the manager that the addr belongs to.
		var accNo uint32
		if accNo, e = scopedMgr.AddrAccount(ns, address); T.Chk(e) {
			return nil, 0, e
		}
		return scopedMgr, accNo, e
	}
	// If we get to this point, then we weren't able to find the address in any of
	// the managers, so we'll exit with an error.
	str := fmt.Sprintf("unable to find key for addr %v", address)
	return nil, 0, managerError(ErrAddressNotFound, str, nil)
}

// ForEachActiveAccountAddress calls the given function with each active address
// of the given account stored in the manager, across all active scopes,
// breaking early on error.
//
// TODO(tuxcanfly): actually return only active addresses
func (m *Manager) ForEachActiveAccountAddress(
	ns walletdb.ReadBucket,
	account uint32, fn func(maddr ManagedAddress) error,
) (e error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for _, scopedMgr := range m.scopedManagers {
		if e = scopedMgr.ForEachActiveAccountAddress(ns, account, fn); E.Chk(e) {
			return e
		}
	}
	return nil
}

// ForEachActiveAddress calls the given function with each active address stored
// in the manager, breaking early on error.
func (m *Manager) ForEachActiveAddress(ns walletdb.ReadBucket, fn func(addr btcaddr.Address) error) (e error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for _, scopedMgr := range m.scopedManagers {
		if e = scopedMgr.ForEachActiveAddress(ns, fn); E.Chk(e) {
			return e
		}
	}
	return nil
}

// ForEachAccountAddress calls the given function with each address of the given
// account stored in the manager, breaking early on error.
func (m *Manager) ForEachAccountAddress(
	ns walletdb.ReadBucket, account uint32,
	fn func(maddr ManagedAddress) error,
) (e error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for _, scopedMgr := range m.scopedManagers {
		if e = scopedMgr.ForEachAccountAddress(ns, account, fn); E.Chk(e) {
			return e
		}
	}
	return nil
}

// ChainParams returns the chain parameters for this address manager.
func (m *Manager) ChainParams() *chaincfg.Params {
	// NOTE: No need for mutex here since the net field does not change after the
	// manager instance is created.
	return m.chainParams
}

// ChangePassphrase changes either the public or private passphrase to the
// provided value depending on the private flag. In order to change the private
// password, the address manager must not be watching-only.
//
// The new passphrase keys are derived using the scrypt parameters in the
// options, so changing the passphrase may be used to bump the computational
// difficulty needed to brute force the passphrase.
func (m *Manager) ChangePassphrase(
	ns walletdb.ReadWriteBucket, oldPassphrase,
	newPassphrase []byte, private bool, config *ScryptOptions,
) (e error) {
	// No private passphrase to change for a watching-only address manager.
	if private && m.watchingOnly {
		return managerError(ErrWatchingOnly, errWatchingOnly, nil)
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// Ensure the provided old passphrase is correct. This check is done using a
	// copy of the appropriate master key depending on the private flag to ensure
	// the current state is not altered. The temp key is cleared when done to avoid
	// leaving a copy in memory.
	var keyName string
	secretKey := snacl.SecretKey{Key: &snacl.CryptoKey{}}
	if private {
		keyName = "private"
		secretKey.Parameters = m.masterKeyPriv.Parameters
	} else {
		keyName = "public"
		secretKey.Parameters = m.masterKeyPub.Parameters
	}
	if e = secretKey.DeriveKey(&oldPassphrase); E.Chk(e) {
		if e == snacl.ErrInvalidPassword {
			str := fmt.Sprintf(
				"invalid passphrase for %s master key", keyName,
			)
			return managerError(ErrWrongPassphrase, str, nil)
		}
		str := fmt.Sprintf("failed to derive %s master key", keyName)
		return managerError(ErrCrypto, str, e)
	}
	defer secretKey.Zero()
	// Generate a new master key from the passphrase which is used to secure the
	// actual secret keys.
	var newMasterKey *snacl.SecretKey
	if newMasterKey, e = newSecretKey(&newPassphrase, config); E.Chk(e) {
		str := "failed to create new master private key"
		return managerError(ErrCrypto, str, e)
	}
	newKeyParams := newMasterKey.Marshal()
	if private {
		// Technically, the locked state could be checked here to only do the decrypts
		// when the address manager is locked as the clear text keys are already
		// available in memory when it is unlocked, but this is not a hot path,
		// decryption is quite fast, and it's less cyclomatic complexity to simply
		// decrypt in either case.
		//
		// Create a new salt that will be used for hashing the new passphrase each
		// unlock.
		var passphraseSalt [saltSize]byte
		if _, e = rand.Read(passphraseSalt[:]); E.Chk(e) {
			str := "failed to read random source for passhprase salt"
			return managerError(ErrCrypto, str, e)
		}
		// Re-encrypt the crypto private key using the new master private key.
		var decPriv []byte
		if decPriv, e = secretKey.Decrypt(m.cryptoKeyPrivEncrypted); E.Chk(e) {
			str := "failed to decrypt crypto private key"
			return managerError(ErrCrypto, str, e)
		}
		var encPriv []byte
		if encPriv, e = newMasterKey.Encrypt(decPriv); E.Chk(e) {
			zero.Bytes(decPriv)
			str := "failed to encrypt crypto private key"
			return managerError(ErrCrypto, str, e)
		}
		zero.Bytes(decPriv)
		// Re-encrypt the crypto script key using the new master private key.
		var decScript []byte
		if decScript, e = secretKey.Decrypt(m.cryptoKeyScriptEncrypted); E.Chk(e) {
			str := "failed to decrypt crypto script key"
			return managerError(ErrCrypto, str, e)
		}
		var encScript []byte
		if encScript, e = newMasterKey.Encrypt(decScript); E.Chk(e) {
			zero.Bytes(decScript)
			str := "failed to encrypt crypto script key"
			return managerError(ErrCrypto, str, e)
		}
		zero.Bytes(decScript)
		// When the manager is locked, ensure the new clear text master key is cleared
		// from memory now that it is no longer needed. If unlocked, create the new
		// passphrase hash with the new passphrase and salt.
		var hashedPassphrase [sha512.Size]byte
		if m.locked {
			newMasterKey.Zero()
		} else {
			saltedPassphrase := append(passphraseSalt[:], newPassphrase...)
			hashedPassphrase = sha512.Sum512(saltedPassphrase)
			zero.Bytes(saltedPassphrase)
		}
		// Save the new keys and netparams to the db in a single transaction.
		if e = putCryptoKeys(ns, nil, encPriv, encScript); E.Chk(e) {
			return maybeConvertDbError(e)
		}
		if e = putMasterKeyParams(ns, nil, newKeyParams); E.Chk(e) {
			return maybeConvertDbError(e)
		}
		// Now that the db has been successfully updated, clear the old key and set the
		// new one.
		copy(m.cryptoKeyPrivEncrypted, encPriv)
		copy(m.cryptoKeyScriptEncrypted, encScript)
		m.masterKeyPriv.Zero() // Clear the old key.
		m.masterKeyPriv = newMasterKey
		m.privPassphraseSalt = passphraseSalt
		m.hashedPrivPassphrase = hashedPassphrase
	} else {
		// Re-encrypt the crypto public key using the new master public key.
		var encryptedPub []byte
		if encryptedPub, e = newMasterKey.Encrypt(m.cryptoKeyPub.Bytes()); E.Chk(e) {
			str := "failed to encrypt crypto public key"
			return managerError(ErrCrypto, str, e)
		}
		// Save the new keys and netparams to the the db in a single
		// transaction.
		if e = putCryptoKeys(ns, encryptedPub, nil, nil); E.Chk(e) {
			return maybeConvertDbError(e)
		}
		if e = putMasterKeyParams(ns, newKeyParams, nil); E.Chk(e) {
			return maybeConvertDbError(e)
		}
		// Now that the db has been successfully updated, clear the old key and set the
		// new one.
		m.masterKeyPub.Zero()
		m.masterKeyPub = newMasterKey
	}
	return nil
}

// ConvertToWatchingOnly converts the current address manager to a locked
// watching-only address manager.
//
// WARNING: This function removes private keys from the existing address manager
// which means they will no longer be available. Typically the caller will make
// a copy of the existing wallet database and modify the copy since otherwise it
// would mean permanent loss of any imported private keys and scripts.
//
// Executing this function on a manager that is already watching-only will have
// no effect.
func (m *Manager) ConvertToWatchingOnly(ns walletdb.ReadWriteBucket) (e error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// Exit now if the manager is already watching-only.
	if m.watchingOnly {
		return nil
	}
	// Remove all private key material and mark the new database as watching only.
	if e = deletePrivateKeys(ns); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	if e = putWatchingOnly(ns, true); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	// Lock the manager to remove all clear text private key material from memory if
	// needed.
	if !m.locked {
		m.lock()
	}
	// This section clears and removes the encrypted private key material that is
	// ordinarily used to unlock the manager. Since the the manager is being
	// converted to watching-only, the encrypted private key material is no longer
	// needed.
	//
	// Clear and remove all of the encrypted acount private keys.
	for _, manager := range m.scopedManagers {
		for _, acctInfo := range manager.acctInfo {
			zero.Bytes(acctInfo.acctKeyEncrypted)
			acctInfo.acctKeyEncrypted = nil
		}
	}
	// Clear and remove encrypted private keys and encrypted scripts from all
	// address entries.
	for _, manager := range m.scopedManagers {
		for _, ma := range manager.addrs {
			switch addr := ma.(type) {
			case *managedAddress:
				zero.Bytes(addr.privKeyEncrypted)
				addr.privKeyEncrypted = nil
			case *scriptAddress:
				zero.Bytes(addr.scriptEncrypted)
				addr.scriptEncrypted = nil
			}
		}
	}
	// Clear and remove encrypted private and script crypto keys.
	zero.Bytes(m.cryptoKeyScriptEncrypted)
	m.cryptoKeyScriptEncrypted = nil
	m.cryptoKeyScript = nil
	zero.Bytes(m.cryptoKeyPrivEncrypted)
	m.cryptoKeyPrivEncrypted = nil
	m.cryptoKeyPriv = nil
	// The master private key is derived from a passphrase when the manager is
	// unlocked, so there is no encrypted version to zero. However, it is no longer
	// needed, so nil it.
	m.masterKeyPriv = nil
	// Mark the manager watching-only.
	m.watchingOnly = true
	return nil
}

// IsLocked returns whether or not the address managed is locked. When it is
// unlocked, the decryption key needed to decrypt private keys used for signing
// is in memory.
func (m *Manager) IsLocked() bool {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return m.isLocked()
}

// isLocked is an internal method returning whether or not the address manager
// is locked via an unprotected read.
//
// NOTE: The caller *MUST* acquire the Manager's mutex before invocation to
// avoid data races.
func (m *Manager) isLocked() bool {
	return m.locked
}

// Lock performs a best try effort to remove and zero all secret keys associated
// with the address manager.
//
// This function will return an error if invoked on a watching-only address
// manager.
func (m *Manager) Lock() (e error) {
	// A watching-only address manager can't be locked.
	if m.watchingOnly {
		return managerError(ErrWatchingOnly, errWatchingOnly, nil)
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// DBError on attempt to lock an already locked manager.
	if m.locked {
		return managerError(ErrLocked, errLocked, nil)
	}
	m.lock()
	return nil
}

// Unlock derives the master private key from the specified passphrase. An
// invalid passphrase will return an error. Otherwise, the derived secret key is
// stored in memory until the address manager is locked. Any failures that occur
// during this function will result in the address manager being locked, even if
// it was already unlocked prior to calling this function.
//
// This function will return an error if invoked on a watching-only address
// manager.
func (m *Manager) Unlock(ns walletdb.ReadBucket, passphrase []byte) (e error) {
	// A watching-only address manager can't be unlocked.
	if m.watchingOnly {
		return managerError(ErrWatchingOnly, errWatchingOnly, nil)
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// Avoid actually unlocking if the manager is already unlocked
	// and the passphrases match.
	if !m.locked {
		saltedPassphrase := append(
			m.privPassphraseSalt[:],
			passphrase...,
		)
		hashedPassphrase := sha512.Sum512(saltedPassphrase)
		zero.Bytes(saltedPassphrase)
		if hashedPassphrase != m.hashedPrivPassphrase {
			m.lock()
			str := "invalid passphrase for master private key"
			return managerError(ErrWrongPassphrase, str, nil)
		}
		return nil
	}
	// Derive the master private key using the provided passphrase.
	if e = m.masterKeyPriv.DeriveKey(&passphrase); E.Chk(e) {
		m.lock()
		if e == snacl.ErrInvalidPassword {
			str := "invalid passphrase for master private key"
			return managerError(ErrWrongPassphrase, str, nil)
		}
		str := "failed to derive master private key"
		return managerError(ErrCrypto, str, e)
	}
	// Use the master private key to decrypt the crypto private key.
	var decryptedKey []byte
	if decryptedKey, e = m.masterKeyPriv.Decrypt(m.cryptoKeyPrivEncrypted); E.Chk(e) {
		m.lock()
		str := "failed to decrypt crypto private key"
		return managerError(ErrCrypto, str, e)
	}
	m.cryptoKeyPriv.CopyBytes(decryptedKey)
	zero.Bytes(decryptedKey)
	// Use the crypto private key to decrypt all of the account private extended
	// keys.
	for _, manager := range m.scopedManagers {
		var acctKeyPriv *hdkeychain.ExtendedKey
		for account, acctInfo := range manager.acctInfo {
			var decrypted []byte
			if decrypted, e = m.cryptoKeyPriv.Decrypt(acctInfo.acctKeyEncrypted); E.Chk(e) {
				m.lock()
				str := fmt.Sprintf("failed to decrypt account %d private key", account)
				return managerError(ErrCrypto, str, e)
			}
			if acctKeyPriv, e = hdkeychain.NewKeyFromString(string(decrypted)); E.Chk(e) {
				zero.Bytes(decrypted)
				m.lock()
				str := fmt.Sprintf("failed to regenerate account %d extended key", account)
				return managerError(ErrKeyChain, str, e)
			}
			zero.Bytes(decrypted)
			acctInfo.acctKeyPriv = acctKeyPriv
		}
		// We'll also derive any private keys that are pending due to them being created
		// while the address manager was locked.
		for _, info := range manager.deriveOnUnlock {
			var addressKey *hdkeychain.ExtendedKey
			if addressKey, e = manager.deriveKeyFromPath(
				ns, info.managedAddr.Account(), info.branch,
				info.index, true,
			); E.Chk(e) {
				m.lock()
				return e
			}
			// It's ok to ignore the error here since it can only fail if the extended key
			// is not private, however it was just derived as a private key.
			privKey, _ := addressKey.ECPrivKey()
			addressKey.Zero()
			privKeyBytes := privKey.Serialize()
			var privKeyEncrypted []byte
			if privKeyEncrypted, e = m.cryptoKeyPriv.Encrypt(privKeyBytes); E.Chk(e) {
				zero.BigInt(privKey.D)
				m.lock()
				str := fmt.Sprintf("failed to encrypt private key for address %s", info.managedAddr.Address())
				return managerError(ErrCrypto, str, e)
			}
			zero.BigInt(privKey.D)
			switch a := info.managedAddr.(type) {
			case *managedAddress:
				a.privKeyEncrypted = privKeyEncrypted
				a.privKeyCT = privKeyBytes
			case *scriptAddress:
			}
			// Avoid re-deriving this key on subsequent unlocks.
			manager.deriveOnUnlock[0] = nil
			manager.deriveOnUnlock = manager.deriveOnUnlock[1:]
		}
	}
	m.locked = false
	saltedPassphrase := append(m.privPassphraseSalt[:], passphrase...)
	m.hashedPrivPassphrase = sha512.Sum512(saltedPassphrase)
	zero.Bytes(saltedPassphrase)
	return nil
}

// ValidateAccountName validates the given account name and returns an error, if
// any.
func ValidateAccountName(name string) (e error) {
	if name == "" {
		str := "accounts may not be named the empty string"
		return managerError(ErrInvalidAccount, str, nil)
	}
	if isReservedAccountName(name) {
		str := "reserved account name"
		return managerError(ErrInvalidAccount, str, nil)
	}
	return nil
}

// selectCryptoKey selects the appropriate crypto key based on the key type. An
// error is returned when an invalid key type is specified or the requested key
// requires the manager to be unlocked when it isn't.
//
// This function MUST be called with the manager lock held for reads.
func (m *Manager) selectCryptoKey(keyType CryptoKeyType) (EncryptorDecryptor, error) {
	if keyType == CKTPrivate || keyType == CKTScript {
		// The manager must be unlocked to work with the private keys.
		if m.locked || m.watchingOnly {
			return nil, managerError(ErrLocked, errLocked, nil)
		}
	}
	var cryptoKey EncryptorDecryptor
	switch keyType {
	case CKTPrivate:
		cryptoKey = m.cryptoKeyPriv
	case CKTScript:
		cryptoKey = m.cryptoKeyScript
	case CKTPublic:
		cryptoKey = m.cryptoKeyPub
	default:
		return nil, managerError(
			ErrInvalidKeyType, "invalid key type",
			nil,
		)
	}
	return cryptoKey, nil
}

// Encrypt in using the crypto key type specified by keyType.
func (m *Manager) Encrypt(keyType CryptoKeyType, in []byte) ([]byte, error) {
	// Encryption must be performed under the manager mutex since the keys are
	// cleared when the manager is locked.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var e error
	var cryptoKey EncryptorDecryptor
	if cryptoKey, e = m.selectCryptoKey(keyType); E.Chk(e) {
		return nil, e
	}
	var encrypted []byte
	if encrypted, e = cryptoKey.Encrypt(in); E.Chk(e) {
		return nil, managerError(ErrCrypto, "failed to encrypt", e)
	}
	return encrypted, nil
}

// Decrypt in using the crypto key type specified by keyType.
func (m *Manager) Decrypt(keyType CryptoKeyType, in []byte) ([]byte, error) {
	// Decryption must be performed under the manager mutex since the keys are
	// cleared when the manager is locked.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var cryptoKey EncryptorDecryptor
	var e error
	if cryptoKey, e = m.selectCryptoKey(keyType); E.Chk(e) {
		return nil, e
	}
	var decrypted []byte
	if decrypted, e = cryptoKey.Decrypt(in); E.Chk(e) {
		return nil, managerError(ErrCrypto, "failed to decrypt", e)
	}
	return decrypted, nil
}

// newManager returns a new locked address manager with the given parameters.
func newManager(
	chainParams *chaincfg.Params, masterKeyPub *snacl.SecretKey,
	masterKeyPriv *snacl.SecretKey, cryptoKeyPub EncryptorDecryptor,
	cryptoKeyPrivEncrypted, cryptoKeyScriptEncrypted []byte, syncInfo *syncState,
	birthday time.Time, privPassphraseSalt [saltSize]byte,
	scopedManagers map[KeyScope]*ScopedKeyManager,
) *Manager {
	m := &Manager{
		chainParams:              chainParams,
		syncState:                *syncInfo,
		locked:                   true,
		birthday:                 birthday,
		masterKeyPub:             masterKeyPub,
		masterKeyPriv:            masterKeyPriv,
		cryptoKeyPub:             cryptoKeyPub,
		cryptoKeyPrivEncrypted:   cryptoKeyPrivEncrypted,
		cryptoKeyPriv:            &cryptoKey{},
		cryptoKeyScriptEncrypted: cryptoKeyScriptEncrypted,
		cryptoKeyScript:          &cryptoKey{},
		privPassphraseSalt:       privPassphraseSalt,
		scopedManagers:           scopedManagers,
		externalAddrSchemas:      make(map[AddressType][]KeyScope),
		internalAddrSchemas:      make(map[AddressType][]KeyScope),
	}
	for _, sMgr := range m.scopedManagers {
		externalType := sMgr.AddrSchema().ExternalAddrType
		internalType := sMgr.AddrSchema().InternalAddrType
		scope := sMgr.Scope()
		m.externalAddrSchemas[externalType] = append(
			m.externalAddrSchemas[externalType], scope,
		)
		m.internalAddrSchemas[internalType] = append(
			m.internalAddrSchemas[internalType], scope,
		)
	}
	return m
}

// deriveCoinTypeKey derives the cointype key which can be used to derive the
// extended key for an account according to the hierarchy described by BIP0044
// given the coin type key.
//
// In particular this is the hierarchical deterministic extended key path:
// m/purpose'/<coin type>'
func deriveCoinTypeKey(
	masterNode *hdkeychain.ExtendedKey,
	scope KeyScope,
) (*hdkeychain.ExtendedKey, error) {
	// Enforce maximum coin type.
	var e error
	if scope.Coin > maxCoinType {
		e = managerError(ErrCoinTypeTooHigh, errCoinTypeTooHigh, nil)
		return nil, e
	}
	// The hierarchy described by BIP0043 is:
	//
	//  m/<purpose>'/*
	//
	// This is further extended by BIP0044 to:
	//
	//  m/44'/<coin type>'/<account>'/<branch>/<address index>
	//
	// However, as this is a generic key store for any family for BIP0044 standards,
	// we'll use the custom scope to govern our key derivation.
	//
	// The branch is 0 for external addresses and 1 for internal addresses. Derive
	// the purpose key as a child of the master node.
	var purpose *hdkeychain.ExtendedKey
	if purpose, e = masterNode.Child(scope.Purpose + hdkeychain.HardenedKeyStart); E.Chk(e) {
		return nil, e
	}
	// Derive the coin type key as a child of the purpose key.
	var coinTypeKey *hdkeychain.ExtendedKey
	if coinTypeKey, e = purpose.Child(scope.Coin + hdkeychain.HardenedKeyStart); E.Chk(e) {
		return nil, e
	}
	return coinTypeKey, nil
}

// deriveAccountKey derives the extended key for an account according to the
// hierarchy described by BIP0044 given the master node.
//
// In particular this is the hierarchical deterministic extended key path:
//
//   m/purpose'/<coin type>'/<account>'
func deriveAccountKey(coinTypeKey *hdkeychain.ExtendedKey, account uint32) (*hdkeychain.ExtendedKey, error) {
	// Enforce maximum account number.
	var er ManagerError
	if account > MaxAccountNum {
		er = managerError(ErrAccountNumTooHigh, errAcctTooHigh, nil)
		return nil, er
	}
	// Derive the account key as a child of the coin type key.
	return coinTypeKey.Child(account + hdkeychain.HardenedKeyStart)
}

// checkBranchKeys ensures deriving the extended keys for the internal and
// external branches given an account key does not result in an invalid child
// error which means the chosen seed is not usable. This conforms to the
// hierarchy described by the BIP0044 family so long as the account key is
// already derived accordingly.
//
// In particular this is the hierarchical deterministic extended key path:
//
//   m/purpose'/<coin type>'/<account>'/<branch>
//
// The branch is 0 for external addresses and 1 for internal addresses.
func checkBranchKeys(acctKey *hdkeychain.ExtendedKey) (e error) {
	// Derive the external branch as the first child of the account key.
	if _, e = acctKey.Child(ExternalBranch); E.Chk(e) {
		return e
	}
	// Derive the external branch as the second child of the account key.
	if _, e = acctKey.Child(InternalBranch); E.Chk(e) {
	}
	return e
}

// loadManager returns a new address manager that results from loading it from
// the passed opened database. The public passphrase is required to decrypt the
// public keys.
func loadManager(
	ns walletdb.ReadBucket, pubPassphrase []byte,
	chainParams *chaincfg.Params,
) (*Manager, error) {
	D.Ln("loading address manager", log.Caller("from", 1))
	// Verify the version is neither too old or too new.
	var version uint32
	var e error
	D.Ln("fetching manager version")
	if version, e = fetchManagerVersion(ns); E.Chk(e) {
		str := "failed to fetch version for update"
		return nil, managerError(ErrDatabase, str, e)
	}
	if version < latestMgrVersion {
		str := "database upgrade required"
		D.Ln(str)
		return nil, managerError(ErrUpgrade, str, nil)
	} else if version > latestMgrVersion {
		str := "database version is greater than latest understood version"
		D.Ln(str)
		return nil, managerError(ErrUpgrade, str, nil)
	}
	// Load whether or not the manager is watching-only from the db.
	var watchingOnly bool
	D.Ln("loading watching only state from db")
	if watchingOnly, e = fetchWatchingOnly(ns); E.Chk(e) {
		return nil, maybeConvertDbError(e)
	}
	// Load the master key netparams from the db.
	var masterKeyPubParams []byte
	var masterKeyPrivParams []byte
	D.Ln("fetching master key params")
	if masterKeyPubParams, masterKeyPrivParams, e = fetchMasterKeyParams(ns); E.Chk(e) {
		return nil, maybeConvertDbError(e)
	}
	// Load the crypto keys from the db.
	var cryptoKeyPubEnc, cryptoKeyPrivEnc, cryptoKeyScriptEnc []byte
	D.Ln("loading crypto keys from wallet db")
	if cryptoKeyPubEnc, cryptoKeyPrivEnc, cryptoKeyScriptEnc, e = fetchCryptoKeys(ns); E.Chk(e) {
		return nil, maybeConvertDbError(e)
	}
	// Load the sync state from the db.
	var syncedTo *BlockStamp
	D.Ln("loading wallet sync state")
	if syncedTo, e = fetchSyncedTo(ns); E.Chk(e) {
		return nil, maybeConvertDbError(e)
	}
	var startBlock *BlockStamp
	D.Ln("fetching start block for wallet")
	if startBlock, e = fetchStartBlock(ns); E.Chk(e) {
		return nil, maybeConvertDbError(e)
	}
	var birthday time.Time
	D.Ln("fetching wallet birthday")
	if birthday, e = fetchBirthday(ns); E.Chk(e) {
		return nil, maybeConvertDbError(e)
	}
	// When not a watching-only manager, set the master private key netparams, but
	// don't derive it now since the manager starts off locked.
	var masterKeyPriv snacl.SecretKey
	if !watchingOnly {
		D.Ln("unmarshalling wallet master private key parameters")
		if e = masterKeyPriv.Unmarshal(masterKeyPrivParams); E.Chk(e) {
			str := "failed to unmarshal master private key"
			return nil, managerError(ErrCrypto, str, e)
		}
	}
	// Derive the master public key using the serialized netparams and provided
	// passphrase.
	var masterKeyPub snacl.SecretKey
	D.Ln("unmarshalling wallet master public key")
	if e = masterKeyPub.Unmarshal(masterKeyPubParams); E.Chk(e) {
		str := "failed to unmarshal master public key"
		return nil, managerError(ErrCrypto, str, e)
	}
	D.F("deriving pub key passphrase key '%s'" , string(pubPassphrase))
	if e = masterKeyPub.DeriveKey(&pubPassphrase); E.Chk(e) {
		str := "invalid passphrase for master public key"
		return nil, managerError(ErrWrongPassphrase, str, nil)
	}
	// Use the master public key to decrypt the crypto public key.
	cryptoKeyPub := &cryptoKey{snacl.CryptoKey{}}
	var cryptoKeyPubCT []byte
	D.Ln("decrypting master public key")
	if cryptoKeyPubCT, e = masterKeyPub.Decrypt(cryptoKeyPubEnc); E.Chk(e) {
		str := "failed to decrypt crypto public key"
		return nil, managerError(ErrCrypto, str, e)
	}
	cryptoKeyPub.CopyBytes(cryptoKeyPubCT)
	zero.Bytes(cryptoKeyPubCT)
	// Create the sync state struct.
	D.Ln("creating new sync state")
	syncInfo := newSyncState(startBlock, syncedTo)
	// Generate private passphrase salt.
	var privPassphraseSalt [saltSize]byte
	D.Ln("generating private passphrase salt")
	if _, e = rand.Read(privPassphraseSalt[:]); E.Chk(e) {
		str := "failed to read random source for passphrase salt"
		return nil, managerError(ErrCrypto, str, e)
	}
	// Next, we'll need to load all known manager scopes from disk. Each scope is on
	// a distinct top-level path within our HD key chain.
	D.Ln("loading all known wallet address manager scopes")
	scopedManagers := make(map[KeyScope]*ScopedKeyManager)
	if e = forEachKeyScope(
		ns, func(scope KeyScope) (e error) {
			scopeSchema, e := fetchScopeAddrSchema(ns, &scope)
			if e != nil {
				return e
			}
			scopedManagers[scope] = &ScopedKeyManager{
				scope:      scope,
				addrSchema: *scopeSchema,
				addrs:      make(map[addrKey]ManagedAddress),
				acctInfo:   make(map[uint32]*accountInfo),
			}
			return nil
		},
	); E.Chk(e) {
		return nil, e
	}
	// Create new address manager with the given parameters. Also, override the
	// defaults for the additional fields which are not specified in the call to new
	// with the values loaded from the database.
	D.Ln("creating new wallet address manager")
	mgr := newManager(
		chainParams, &masterKeyPub, &masterKeyPriv,
		cryptoKeyPub, cryptoKeyPrivEnc, cryptoKeyScriptEnc, syncInfo,
		birthday, privPassphraseSalt, scopedManagers,
	)
	mgr.watchingOnly = watchingOnly
	for _, scopedManager := range scopedManagers {
		scopedManager.rootManager = mgr
	}
	D.Ln("successfully created new wallet address manager")
	return mgr, nil
}

// Open loads an existing address manager from the given namespace. The public
// passphrase is required to decrypt the public keys used to protect the public
// information such as addresses. This is important since access to BIP0032
// extended keys means it is possible to generate all future addresses.
//
// If a config structure is passed to the function, that configuration will
// override the defaults.
//
// A ManagerError with an error code of ErrNoExist will be returned if the
// passed manager does not exist in the specified namespace.
func Open(
	ns walletdb.ReadBucket, pubPassphrase []byte,
	chainParams *chaincfg.Params,
) (*Manager, error) {
	D.Ln("opening address manager")
	// Return an error if the manager has NOT already been created in the
	// given database namespace.
	exists := managerExists(ns)
	if !exists {
		str := "the specified address manager does not exist"
		return nil, managerError(ErrNoExist, str, nil)
	}
	return loadManager(ns, pubPassphrase, chainParams)
}

// DoUpgrades performs any necessary upgrades to the address manager contained
// in the wallet database, namespaced by the top level bucket key namespaceKey.
func DoUpgrades(
	db walletdb.DB, namespaceKey []byte, pubPassphrase []byte,
	chainParams *chaincfg.Params, cbs *OpenCallbacks,
) (e error) {
	return upgradeManager(db, namespaceKey, pubPassphrase, chainParams, cbs)
}

// createManagerKeyScope creates a new key scoped for a target manager's scope.
// This partitions key derivation for a particular purpose+coin tuple, allowing
// multiple address derivation schemes to be maintained concurrently.
func createManagerKeyScope(
	ns walletdb.ReadWriteBucket,
	scope KeyScope, root *hdkeychain.ExtendedKey,
	cryptoKeyPub, cryptoKeyPriv EncryptorDecryptor,
) (e error) {
	// Derive the cointype key according to the passed scope.
	var coinTypeKeyPriv *hdkeychain.ExtendedKey
	if coinTypeKeyPriv, e = deriveCoinTypeKey(root, scope); E.Chk(e) {
		str := "failed to derive cointype extended key"
		return managerError(ErrKeyChain, str, e)
	}
	defer coinTypeKeyPriv.Zero()
	// Derive the account key for the first account according our BIP0044-like
	// derivation.
	var acctKeyPriv *hdkeychain.ExtendedKey
	if acctKeyPriv, e = deriveAccountKey(coinTypeKeyPriv, 0); E.Chk(e) {
		// The seed is unusable if the any of the children in the required hierarchy
		// can't be derived due to invalid child.
		if e == hdkeychain.ErrInvalidChild {
			str := "the provided seed is unusable"
			return managerError(
				ErrKeyChain, str,
				hdkeychain.ErrUnusableSeed,
			)
		}
		return e
	}
	// Ensure the branch keys can be derived for the provided seed according to our
	// BIP0044-like derivation.
	if e = checkBranchKeys(acctKeyPriv); E.Chk(e) {
		// The seed is unusable if the any of the children in the required hierarchy
		// can't be derived due to invalid child.
		if e == hdkeychain.ErrInvalidChild {
			str := "the provided seed is unusable"
			return managerError(
				ErrKeyChain, str,
				hdkeychain.ErrUnusableSeed,
			)
		}
		return e
	}
	// The address manager needs the public extended key for the account.
	var acctKeyPub *hdkeychain.ExtendedKey
	if acctKeyPub, e = acctKeyPriv.Neuter(); E.Chk(e) {
		str := "failed to convert private key for account 0"
		return managerError(ErrKeyChain, str, e)
	}
	// Encrypt the cointype keys with the associated crypto keys.
	var coinTypeKeyPub *hdkeychain.ExtendedKey
	if coinTypeKeyPub, e = coinTypeKeyPriv.Neuter(); E.Chk(e) {
		str := "failed to convert cointype private key"
		return managerError(ErrKeyChain, str, e)
	}
	var coinTypePubEnc []byte
	if coinTypePubEnc, e = cryptoKeyPub.Encrypt([]byte(coinTypeKeyPub.String())); E.Chk(e) {
		str := "failed to encrypt cointype public key"
		return managerError(ErrCrypto, str, e)
	}
	var coinTypePrivEnc []byte
	if coinTypePrivEnc, e = cryptoKeyPriv.Encrypt([]byte(coinTypeKeyPriv.String())); E.Chk(e) {
		str := "failed to encrypt cointype private key"
		return managerError(ErrCrypto, str, e)
	}
	// Encrypt the default account keys with the associated crypto keys.
	var acctPubEnc []byte
	if acctPubEnc, e = cryptoKeyPub.Encrypt([]byte(acctKeyPub.String())); E.Chk(e) {
		str := "failed to  encrypt public key for account 0"
		return managerError(ErrCrypto, str, e)
	}
	var acctPrivEnc []byte
	if acctPrivEnc, e = cryptoKeyPriv.Encrypt([]byte(acctKeyPriv.String())); E.Chk(e) {
		str := "failed to encrypt private key for account 0"
		return managerError(ErrCrypto, str, e)
	}
	// Save the encrypted cointype keys to the database.
	if e = putCoinTypeKeys(ns, &scope, coinTypePubEnc, coinTypePrivEnc); E.Chk(e) {
		return e
	}
	// Save the information for the default account to the database.
	if e = putAccountInfo(
		ns, &scope, DefaultAccountNum, acctPubEnc, acctPrivEnc, 0, 0,
		defaultAccountName,
	); E.Chk(e) {
		return e
	}
	return putAccountInfo(
		ns, &scope, ImportedAddrAccount, nil, nil, 0, 0,
		ImportedAddrAccountName,
	)
}

// Create creates a new address manager in the given namespace. The seed must
// conform to the standards described in hdkeychain.NewMaster and will be used
// to create the master root node from which all hierarchical deterministic
// addresses are derived. This allows all chained addresses in the address
// manager to be recovered by using the same seed.
//
// All private and public keys and information are protected by secret keys
// derived from the provided private and public passphrases. The public
// passphrase is required on subsequent opens of the address manager, and the
// private passphrase is required to unlock the address manager in order to gain
// access to any private keys and information.
//
// If a config structure is passed to the function, that configuration will
// override the defaults.
//
// A ManagerError with an error code of ErrAlreadyExists will be returned the
// address manager already exists in the specified namespace.
func Create(
	ns walletdb.ReadWriteBucket, seed, pubPassphrase, privPassphrase []byte,
	chainParams *chaincfg.Params, config *ScryptOptions,
	birthday time.Time,
) (e error) {
	// Return an error if the manager has already been created in the given database
	// namespace.
	exists := managerExists(ns)
	if exists {
		return managerError(ErrAlreadyExists, errAlreadyExists, nil)
	}
	// Ensure the private passphrase is not empty.
	if len(privPassphrase) == 0 {
		str := "private passphrase may not be empty"
		return managerError(ErrEmptyPassphrase, str, nil)
	}
	// Perform the initial bucket creation and database namespace setup.
	if e = createManagerNS(ns, ScopeAddrMap); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	if config == nil {
		config = &DefaultScryptOptions
	}
	// Generate new master keys. These master keys are used to protect the crypto
	// keys that will be generated next.
	var masterKeyPub *snacl.SecretKey
	if masterKeyPub, e = newSecretKey(&pubPassphrase, config); E.Chk(e) {
		str := "failed to master public key"
		return managerError(ErrCrypto, str, e)
	}
	var masterKeyPriv *snacl.SecretKey
	if masterKeyPriv, e = newSecretKey(&privPassphrase, config); E.Chk(e) {
		str := "failed to master private key"
		return managerError(ErrCrypto, str, e)
	}
	defer masterKeyPriv.Zero()
	// Generate the private passphrase salt. This is used when hashing passwords to
	// detect whether an unlock can be avoided when the manager is already unlocked.
	var privPassphraseSalt [saltSize]byte
	if _, e = rand.Read(privPassphraseSalt[:]); E.Chk(e) {
		str := "failed to read random source for passphrase salt"
		return managerError(ErrCrypto, str, e)
	}
	// Generate new crypto public, private, and script keys. These keys are used to
	// protect the actual public and private data such as addresses, extended keys,
	// and scripts.
	var cryptoKeyPub EncryptorDecryptor
	if cryptoKeyPub, e = newCryptoKey(); E.Chk(e) {
		str := "failed to generate crypto public key"
		return managerError(ErrCrypto, str, e)
	}
	var cryptoKeyPriv EncryptorDecryptor
	if cryptoKeyPriv, e = newCryptoKey(); E.Chk(e) {
		str := "failed to generate crypto private key"
		return managerError(ErrCrypto, str, e)
	}
	defer cryptoKeyPriv.Zero()
	var cryptoKeyScript EncryptorDecryptor
	if cryptoKeyScript, e = newCryptoKey(); E.Chk(e) {
		str := "failed to generate crypto script key"
		return managerError(ErrCrypto, str, e)
	}
	defer cryptoKeyScript.Zero()
	// Encrypt the crypto keys with the associated master keys.
	var cryptoKeyPubEnc []byte
	if cryptoKeyPubEnc, e = masterKeyPub.Encrypt(cryptoKeyPub.Bytes()); E.Chk(e) {
		str := "failed to encrypt crypto public key"
		return managerError(ErrCrypto, str, e)
	}
	var cryptoKeyPrivEnc []byte
	if cryptoKeyPrivEnc, e = masterKeyPriv.Encrypt(cryptoKeyPriv.Bytes()); E.Chk(e) {
		str := "failed to encrypt crypto private key"
		return managerError(ErrCrypto, str, e)
	}
	var cryptoKeyScriptEnc []byte
	if cryptoKeyScriptEnc, e = masterKeyPriv.Encrypt(cryptoKeyScript.Bytes()); E.Chk(e) {
		str := "failed to encrypt crypto script key"
		return managerError(ErrCrypto, str, e)
	}
	// Use the genesis block for the passed chain as the created at block for the
	// default.
	createdAt := &BlockStamp{Hash: *chainParams.GenesisHash, Height: 0}
	// Create the initial sync state.
	syncInfo := newSyncState(createdAt, createdAt)
	// Save the master key netparams to the database.
	pubParams := masterKeyPub.Marshal()
	privParams := masterKeyPriv.Marshal()
	if e = putMasterKeyParams(ns, pubParams, privParams); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	// Generate the BIP0044 HD key structure to ensure the provided seed can
	// generate the required structure with no issues. Derive the master extended
	// key from the seed.
	var rootKey *hdkeychain.ExtendedKey
	if rootKey, e = hdkeychain.NewMaster(seed, chainParams); E.Chk(e) {
		str := "failed to derive master extended key"
		return managerError(ErrKeyChain, str, e)
	}
	var rootPubKey *hdkeychain.ExtendedKey
	if rootPubKey, e = rootKey.Neuter(); E.Chk(e) {
		str := "failed to neuter master extended key"
		return managerError(ErrKeyChain, str, e)
	}
	// Next, for each registers default manager scope, we'll create the hardened
	// cointype key for it, as well as the first default account.
	for _, defaultScope := range DefaultKeyScopes {
		if e = createManagerKeyScope(
			ns, defaultScope, rootKey, cryptoKeyPub, cryptoKeyPriv,
		); E.Chk(e) {
			return maybeConvertDbError(e)
		}
	}
	// Before we proceed, we'll also store the root master private key within the
	// database in an encrypted format. This is required as in the future, we may
	// need to create additional scoped key managers.
	var masterHDPrivKeyEnc []byte
	if masterHDPrivKeyEnc, e = cryptoKeyPriv.Encrypt([]byte(rootKey.String())); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	var masterHDPubKeyEnc []byte
	if masterHDPubKeyEnc, e = cryptoKeyPub.Encrypt([]byte(rootPubKey.String())); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	if e = putMasterHDKeys(ns, masterHDPrivKeyEnc, masterHDPubKeyEnc); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	// Save the encrypted crypto keys to the database.
	if e = putCryptoKeys(
		ns, cryptoKeyPubEnc, cryptoKeyPrivEnc,
		cryptoKeyScriptEnc,
	); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	// Save the fact this is not a watching-only address manager to the database.
	if e = putWatchingOnly(ns, false); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	// Save the initial synced to state.
	if e = putSyncedTo(ns, &syncInfo.syncedTo); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	if e = putStartBlock(ns, &syncInfo.startBlock); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	// Use 48 hours as margin of safety for wallet birthday.
	return putBirthday(ns, birthday.Add(-48*time.Hour))
}