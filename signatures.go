package steward

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type signature string

// allowedSignatures is the structure for reading and writing from
// the signatures map. It holds a mutex to use when interacting with
// the map.
type signatures struct {
	// allowed is a map for holding all the allowed signatures.
	allowed map[signature]struct{}
	mu      sync.Mutex

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

func newSignatures(configuration *Configuration, errorKernel *errorKernel) *signatures {
	s := signatures{
		allowed:       make(map[signature]struct{}),
		configuration: configuration,
		errorKernel:   errorKernel,
	}

	// Set the signing key paths.
	s.SignKeyFolder = filepath.Join(configuration.ConfigFolder, "signing")
	s.SignKeyPrivateKeyPath = filepath.Join(s.SignKeyFolder, "private.key")
	s.SignKeyPublicKeyPath = filepath.Join(s.SignKeyFolder, "public.key")

	err := s.loadSigningKeys()
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	return &s
}

// loadSigningKeys will try to load the ed25519 signing keys. If the
// files are not found new keys will be generated and written to disk.
func (s *signatures) loadSigningKeys() error {
	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(s.SignKeyFolder); os.IsNotExist(err) {
		err := os.MkdirAll(s.SignKeyFolder, 0700)
		if err != nil {
			er := fmt.Errorf("error: failed to create directory for signing keys : %v", err)
			return er
		}

	}

	// Check if there already are any keys in the etc folder.
	foundKey := false

	if _, err := os.Stat(s.SignKeyPublicKeyPath); !os.IsNotExist(err) {
		foundKey = true
	}
	if _, err := os.Stat(s.SignKeyPrivateKeyPath); !os.IsNotExist(err) {
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
		err = s.writeSigningKey(s.SignKeyPublicKeyPath, pubB64string)
		if err != nil {
			return err
		}

		// Write private key to file.
		err = s.writeSigningKey(s.SignKeyPrivateKeyPath, privB64string)
		if err != nil {
			return err
		}

		// Also store the keys in the processes structure so we can
		// reference them from there when we need them.
		s.SignPublicKey = pub
		s.SignPrivateKey = priv

		er := fmt.Errorf("info: no signing keys found, generating new keys")
		log.Printf("%v\n", er)

		// We got the new generated keys now, so we can return.
		return nil
	}

	// Key files found, load them into the processes struct fields.
	pubKey, _, err := s.readKeyFile(s.SignKeyPublicKeyPath)
	if err != nil {
		return err
	}
	s.SignPublicKey = pubKey

	privKey, _, err := s.readKeyFile(s.SignKeyPrivateKeyPath)
	if err != nil {
		return err
	}
	s.SignPublicKey = pubKey
	s.SignPrivateKey = privKey

	return nil
}

// writeSigningKey will write the base64 encoded signing key to file.
func (s *signatures) writeSigningKey(realPath string, keyB64 string) error {
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
func (s *signatures) readKeyFile(keyFile string) (ed2519key []byte, b64Key []byte, err error) {
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
func (s *signatures) verifySignature(m Message) bool {
	fmt.Printf(" * DEBUG: verifySignature, method: %v ,s contains: %v\n", m.Method, s)
	if s.configuration.AllowEmptySignature {
		fmt.Printf(" * DEBUG: verifySignature: AllowEmptySignature set to TRUE\n")
		return true
	}

	// Verify if the signature matches.
	argsStringified := argsToString(m.MethodArgs)
	ok := ed25519.Verify(s.SignPublicKey, []byte(argsStringified), m.ArgSignature)

	fmt.Printf(" * DEBUG: verifySignature, result: %v, fromNode: %v, method: %v, signature: %s\n", ok, m.FromNode, m.Method, m.ArgSignature)

	return ok
}
