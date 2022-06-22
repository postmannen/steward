package steward

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// nodeAuth is the structure that holds both keys and acl's
// that the running steward node shall use for authorization.
//It holds a mutex to use when interacting with the map.
type nodeAuth struct {
	// ACL that defines where a node is allowed to recieve from.
	nodeAcl *nodeAcl

	// All the public keys for nodes a node is allowed to receive from.
	publicKeys *publicKeys

	// Full path to the signing keys folder
	SignKeyFolder string
	// Full path to private signing key.
	SignKeyPrivateKeyPath string
	// Full path to public signing key.
	SignKeyPublicKeyPath string

	// private key for ed25519 signing.
	SignPrivateKey []byte
	// public key for ed25519 signing.
	SignPublicKey []byte

	configuration *Configuration

	errorKernel *errorKernel
}

func newNodeAuth(configuration *Configuration, errorKernel *errorKernel) *nodeAuth {
	n := nodeAuth{
		nodeAcl:       newNodeAcl(configuration),
		publicKeys:    newPublicKeys(configuration),
		configuration: configuration,
		errorKernel:   errorKernel,
	}

	// Set the signing key paths.
	n.SignKeyFolder = filepath.Join(configuration.ConfigFolder, "signing")
	n.SignKeyPrivateKeyPath = filepath.Join(n.SignKeyFolder, "private.key")
	n.SignKeyPublicKeyPath = filepath.Join(n.SignKeyFolder, "public.key")

	err := n.loadSigningKeys()
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	return &n
}

// --------------------- ACL ---------------------

type aclAndHash struct {
	Acl  map[Node]map[command]struct{}
	Hash [32]byte
}

func newAclAndHash() aclAndHash {
	a := aclAndHash{
		Acl: make(map[Node]map[command]struct{}),
	}

	return a
}

type nodeAcl struct {
	// allowed is a map for holding all the allowed signatures.
	aclAndHash aclAndHash
	filePath   string
	mu         sync.Mutex
}

func newNodeAcl(c *Configuration) *nodeAcl {
	n := nodeAcl{
		aclAndHash: newAclAndHash(),
		filePath:   filepath.Join(c.DatabaseFolder, "node_aclmap.txt"),
	}

	err := n.loadFromFile()
	if err != nil {
		log.Printf("error: loading acl's from file: %v\n", err)
		// os.Exit(1)
	}

	return &n
}

// loadFromFile will try to load all the currently stored acl's from file,
// and return the error if it fails.
// If no file is found a nil error is returned.
func (n *nodeAcl) loadFromFile() error {
	if _, err := os.Stat(n.filePath); os.IsNotExist(err) {
		// Just logging the error since it is not crucial that a key file is missing,
		// since a new one will be created on the next update.
		log.Printf("no acl file found at %v\n", n.filePath)
		return nil
	}

	fh, err := os.OpenFile(n.filePath, os.O_RDONLY, 0600)
	if err != nil {
		return fmt.Errorf("error: failed to open acl file: %v", err)
	}
	defer fh.Close()

	b, err := io.ReadAll(fh)
	if err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	err = json.Unmarshal(b, &n.aclAndHash)
	if err != nil {
		return err
	}

	log.Printf("\n ***** DEBUG: Loaded existing acl's from file: %v\n\n", n.aclAndHash.Hash)

	return nil
}

// saveToFile will save the acl to file for persistent storage.
// An error is returned if it fails.
func (n *nodeAcl) saveToFile() error {
	fh, err := os.OpenFile(n.filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("error: failed to acl file: %v", err)
	}
	defer fh.Close()

	n.mu.Lock()
	defer n.mu.Unlock()

	enc := json.NewEncoder(fh)
	enc.SetEscapeHTML(false)
	enc.Encode(n.aclAndHash)

	// HERE
	// b, err := json.Marshal(n.aclAndHash)
	if err != nil {
		return err
	}

	// _, err = fh.Write(b)
	// if err != nil {
	// 	return err
	// }

	return nil
}

// --------------------- KEYS ---------------------

type keysAndHash struct {
	Keys map[Node][]byte
	Hash [32]byte
}

func newKeysAndHash() *keysAndHash {
	kh := keysAndHash{
		Keys: make(map[Node][]byte),
	}
	return &kh
}

type publicKeys struct {
	keysAndHash *keysAndHash
	mu          sync.Mutex
	filePath    string
}

func newPublicKeys(c *Configuration) *publicKeys {
	p := publicKeys{
		keysAndHash: newKeysAndHash(),
		filePath:    filepath.Join(c.DatabaseFolder, "publickeys.txt"),
	}

	err := p.loadFromFile()
	if err != nil {
		log.Printf("error: loading public keys from file: %v\n", err)
		// os.Exit(1)
	}

	return &p
}

// loadFromFile will try to load all the currently stored public keys from file,
// and return the error if it fails.
// If no file is found a nil error is returned.
func (p *publicKeys) loadFromFile() error {
	if _, err := os.Stat(p.filePath); os.IsNotExist(err) {
		// Just logging the error since it is not crucial that a key file is missing,
		// since a new one will be created on the next update.
		log.Printf("no public keys file found at %v\n", p.filePath)
		return nil
	}

	fh, err := os.OpenFile(p.filePath, os.O_RDONLY, 0600)
	if err != nil {
		return fmt.Errorf("error: failed to open public keys file: %v", err)
	}
	defer fh.Close()

	b, err := io.ReadAll(fh)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	err = json.Unmarshal(b, &p.keysAndHash)
	if err != nil {
		return err
	}

	log.Printf("\n ***** DEBUG: Loaded existing keys from file: %v\n\n", p.keysAndHash.Hash)

	return nil
}

// saveToFile will save all the public kets to file for persistent storage.
// An error is returned if it fails.
func (p *publicKeys) saveToFile() error {
	fh, err := os.OpenFile(p.filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("error: failed to open public keys file: %v", err)
	}
	defer fh.Close()

	p.mu.Lock()
	defer p.mu.Unlock()
	b, err := json.Marshal(p.keysAndHash)
	if err != nil {
		return err
	}

	_, err = fh.Write(b)
	if err != nil {
		return err
	}

	return nil
}

// loadSigningKeys will try to load the ed25519 signing keys. If the
// files are not found new keys will be generated and written to disk.
func (n *nodeAuth) loadSigningKeys() error {
	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(n.SignKeyFolder); os.IsNotExist(err) {
		err := os.MkdirAll(n.SignKeyFolder, 0700)
		if err != nil {
			er := fmt.Errorf("error: failed to create directory for signing keys : %v", err)
			return er
		}

	}

	// Check if there already are any keys in the etc folder.
	foundKey := false

	if _, err := os.Stat(n.SignKeyPublicKeyPath); !os.IsNotExist(err) {
		foundKey = true
	}
	if _, err := os.Stat(n.SignKeyPrivateKeyPath); !os.IsNotExist(err) {
		foundKey = true
	}

	// If no keys where found generete a new pair, load them into the
	// processes struct fields, and write them to disk.
	if !foundKey {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			er := fmt.Errorf("error: failed to generate ed25519 keys for signing: %v", err)
			return er
		}
		pubB64string := base64.RawStdEncoding.EncodeToString(pub)
		privB64string := base64.RawStdEncoding.EncodeToString(priv)

		// Write public key to file.
		err = n.writeSigningKey(n.SignKeyPublicKeyPath, pubB64string)
		if err != nil {
			return err
		}

		// Write private key to file.
		err = n.writeSigningKey(n.SignKeyPrivateKeyPath, privB64string)
		if err != nil {
			return err
		}

		// Also store the keys in the processes structure so we can
		// reference them from there when we need them.
		n.SignPublicKey = pub
		n.SignPrivateKey = priv

		er := fmt.Errorf("info: no signing keys found, generating new keys")
		log.Printf("%v\n", er)

		// We got the new generated keys now, so we can return.
		return nil
	}

	// Key files found, load them into the processes struct fields.
	pubKey, _, err := n.readKeyFile(n.SignKeyPublicKeyPath)
	if err != nil {
		return err
	}
	n.SignPublicKey = pubKey

	privKey, _, err := n.readKeyFile(n.SignKeyPrivateKeyPath)
	if err != nil {
		return err
	}
	n.SignPublicKey = pubKey
	n.SignPrivateKey = privKey

	return nil
}

// writeSigningKey will write the base64 encoded signing key to file.
func (n *nodeAuth) writeSigningKey(realPath string, keyB64 string) error {
	fh, err := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: failed to open key file for writing: %v", err)
		return er
	}
	defer fh.Close()

	_, err = fh.Write([]byte(keyB64))
	if err != nil {
		er := fmt.Errorf("error: failed to write key to file: %v", err)
		return er
	}

	return nil
}

// readKeyFile will take the path of a key file as input, read the base64
// encoded data, decode the data. It will return the raw data as []byte,
// the base64 encoded data, and any eventual error.
func (n *nodeAuth) readKeyFile(keyFile string) (ed2519key []byte, b64Key []byte, err error) {
	fh, err := os.Open(keyFile)
	if err != nil {
		er := fmt.Errorf("error: failed to open key file: %v", err)
		return nil, nil, er
	}
	defer fh.Close()

	b, err := ioutil.ReadAll(fh)
	if err != nil {
		er := fmt.Errorf("error: failed to read key file: %v", err)
		return nil, nil, er
	}

	key, err := base64.RawStdEncoding.DecodeString(string(b))
	if err != nil {
		er := fmt.Errorf("error: failed to base64 decode key data: %v", err)
		return nil, nil, er
	}

	return key, b, nil
}

// verifySignature
func (n *nodeAuth) verifySignature(m Message) bool {
	// NB: Only enable signature checking for REQCliCommand for now.
	if m.Method != REQCliCommand {
		log.Printf(" * DEBUG: verifySignature,not REQCliCommand and will not do signature check, method: %v\n", m.Method)
		return true
	}

	// Verify if the signature matches.
	argsStringified := argsToString(m.MethodArgs)
	var ok bool

	err := func() error {
		n.publicKeys.mu.Lock()
		pubKey := n.publicKeys.keysAndHash.Keys[m.FromNode]
		if len(pubKey) != 32 {
			err := fmt.Errorf("DEBUG: Length of publicKey: %v", len(pubKey))
			return err
		}

		ok = ed25519.Verify(pubKey, []byte(argsStringified), m.ArgSignature)
		n.publicKeys.mu.Unlock()

		return nil
	}()

	if err != nil {
		log.Printf("%v\n", err)
	}

	log.Printf("info: verifySignature, result: %v, fromNode: %v, method: %v\n", ok, m.FromNode, m.Method)

	return ok
}

// verifyAcl
func (n *nodeAuth) verifyAcl(m Message) bool {
	// NB: Only enable acl checking for REQCliCommand for now.
	if m.Method != REQCliCommand {
		log.Printf(" * DEBUG: verifyAcl: not REQCliCommand and will not do acl check, method: %v\n", m.Method)
		return true
	}

	argsStringified := argsToString(m.MethodArgs)

	// Verify if the command matches the one in the acl map.
	n.nodeAcl.mu.Lock()
	defer n.nodeAcl.mu.Unlock()

	cmdMap, ok := n.nodeAcl.aclAndHash.Acl[m.FromNode]
	if !ok {
		log.Printf(" * DEBUG: verifyAcl: The fromNode=%v was not found in the acl\n", m.FromNode)
		return false
	}

	_, ok = cmdMap[command("*")]
	if ok {
		log.Printf(" * DEBUG: verifyAcl: The acl said \"*\", all commands allowed from node=%v\n", m.FromNode)
		return true
	}

	_, ok = cmdMap[command(argsStringified)]
	if !ok {
		log.Printf(" * DEBUG: verifyAcl: The command=%v was NOT FOUND in the acl\n", m.MethodArgs)
		return false
	}

	log.Printf(" * DEBUG: The command was FOUND in the acl, verifyAcl, result: %v, fromNode: %v, method: %v\n", ok, m.FromNode, m.Method)

	return true
}

// argsToString takes args in the format of []string and returns a string.
func argsToString(args []string) string {
	return strings.Join(args, " ")
}
